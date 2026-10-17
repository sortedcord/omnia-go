package sim

import (
	"context"
	"fmt"
	"strings"
	"time"

	"omnia.adityagupta.dev/pkg/actor"
	"omnia.adityagupta.dev/pkg/llm"
)

// PerceiverInfo provides the physical presence and focus context of an actor to the Broadcaster.
type PerceiverInfo struct {
	ID           EntityID `json:"id"`
	Name         string   `json:"name"`
	SpaceName    string   `json:"space_name"`
	CarriedItems []string `json:"carried_items,omitempty"`
	ActionTaken  string   `json:"action_taken,omitempty"`
}

// ActorActionResult summarizes an actor's turn outcome for perception synthesis.
type ActorActionResult struct {
	ActorID    EntityID         `json:"actor_id"`
	ActorName  string           `json:"actor_name"`
	ActionType actor.ActionType `json:"action_type"`
	ActionText string           `json:"action_text"`
	Verdict    string           `json:"verdict"` // as_intended, altered, preempted
}

// BroadcastInput contains the objective resolution and actor spatial context for a scene.
type BroadcastInput struct {
	SceneID       SceneID
	Tick          int
	Timestamp     string
	Resolution    string
	ActionResults []ActorActionResult
	Actors        []PerceiverInfo
}

// BroadcastResult delivers individualized, subjective sensory text for each actor in the scene.
type BroadcastResult struct {
	SceneID     SceneID             `json:"scene_id"`
	Tick        int                 `json:"tick"`
	Perceptions map[EntityID]string `json:"perceptions"` // actorID -> second-person subjective perception
	Usage       llm.Usage           `json:"-"`
	Duration    time.Duration       `json:"-"`
}

// Broadcaster translates an Arbiter's objective resolution into individualized sensory perceptions.
type Broadcaster interface {
	Broadcast(ctx context.Context, in BroadcastInput) (BroadcastResult, error)
}

// LLMBroadcaster implements Broadcaster using chat completions.
type LLMBroadcaster struct {
	client llm.Client
	model  string
}

// NewLLMBroadcaster creates an LLM-backed scene broadcaster.
func NewLLMBroadcaster(client llm.Client, model string) *LLMBroadcaster {
	if model == "" {
		model = "gemini-3.5-flash-lite"
	}
	return &LLMBroadcaster{
		client: client,
		model:  model,
	}
}

type broadcastWireResponse struct {
	Perceptions map[string]string `json:"perceptions"`
}

// Broadcast synthesizes per-actor subjective perceptions for all actors in the scene.
func (b *LLMBroadcaster) Broadcast(ctx context.Context, in BroadcastInput) (BroadcastResult, error) {
	start := time.Now()
	res := BroadcastResult{
		SceneID:     in.SceneID,
		Tick:        in.Tick,
		Perceptions: make(map[EntityID]string),
	}

	if len(in.Actors) == 0 {
		return res, nil
	}

	systemPrompt := `You are the SCENE PERCEPTION BROADCASTER.
Your role is to translate the Arbiter's objective ground truth into individualized, subjective sensory perceptions for each actor present in the scene.

OUTPUT REQUIREMENT:
You MUST respond ONLY with a valid JSON object matching this schema:
{
  "perceptions": {
    "<exact_actor_id_1>": "<second-person sensory description of what this actor experienced, saw, heard, and felt>",
    "<exact_actor_id_2>": "<second-person sensory description tailored to this actor's perspective>"
  }
}

BROADCASTING RULES:
1. Pure Second-Person Subjectivity:
   - Write each actor's perception in second person ("You see...", "You hear...", "You feel...").
   - If an actor took an action, include their personal physical experience of their own attempt and its outcome.
   - For onlookers, describe what they physically witnessed or heard from their vantage point.
   - Tailor to spatial context: An actor sitting close sees subtle facial expressions and hears spoken words clearly; an actor across the counter or in an adjacent room hears muffled sounds, clatter, or raised voices.
2. No Meta-Leaking:
   - Do NOT leak internal thinking tokens or unseen intentions. Only report what was physically perceptible.
   - Do NOT mention verdicts, dice rolls, or simulation mechanics.
3. Keep it Grounded:
   - Be concise, atmospheric, and natural (1-3 sentences per actor).`

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("SCENE: %s | TICK: %d | TIME: %s\n\n", in.SceneID, in.Tick, in.Timestamp))
	sb.WriteString(fmt.Sprintf("OBJECTIVE SCENE RESOLUTION (Ground Truth):\n%s\n\n", in.Resolution))

	if len(in.ActionResults) > 0 {
		sb.WriteString("ACTIONS ATTEMPTED IN THIS TICK:\n")
		for _, ar := range in.ActionResults {
			sb.WriteString(fmt.Sprintf("- %s (%s, verdict: %s): %q\n", ar.ActorName, ar.ActionType, ar.Verdict, ar.ActionText))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("ACTORS IN THIS SCENE (Provide a perception entry for each exact ID):\n")
	for _, a := range in.Actors {
		actionNote := ""
		if a.ActionTaken != "" {
			actionNote = fmt.Sprintf(" | action: %q", a.ActionTaken)
		}
		sb.WriteString(fmt.Sprintf("- Actor ID: %q | Name: %s | Space: %s%s\n", a.ID, a.Name, a.SpaceName, actionNote))
	}
	sb.WriteString("\nSynthesize per-actor perceptions:")

	req := llm.ChatRequest{
		Model: b.model,
		Messages: []llm.Message{
			{Role: llm.System, Content: systemPrompt},
			{Role: llm.User, Content: sb.String()},
		},
		ResponseFormat:   llm.JSONObject(),
		ReasoningEffort:  llm.ReasoningLow,
		IncludeReasoning: true,
	}

	resp, err := b.client.ChatCompletion(ctx, req)
	if err != nil {
		return res, fmt.Errorf("broadcaster: request failed: %w", err)
	}

	res.Duration = time.Since(start)
	res.Usage = resp.Usage

	var wire broadcastWireResponse
	if err := llm.DecodeJSON(resp.Content, resp.ReasoningContent, &wire); err != nil {
		return res, fmt.Errorf("broadcaster: invalid perception JSON: %w", err)
	}

	for _, a := range in.Actors {
		text, ok := wire.Perceptions[string(a.ID)]
		if !ok || (a.ActionTaken != "" && strings.TrimSpace(text) == "") {
			return res, fmt.Errorf("broadcaster: missing perception for actor %q", a.ID)
		}
		res.Perceptions[a.ID] = strings.TrimSpace(text)
	}
	// Ignore unrequested actor IDs. Empty bystander perceptions are legitimate.

	return res, nil
}

// minimalPerceptions is the safe fallback when no broadcaster result is available.
// Each actor only learns what they themselves attempted and its verdict, never the
// objective resolution, so ground truth cannot leak.
func minimalPerceptions(actors []PerceiverInfo, results []ActorActionResult) map[EntityID]string {
	out := make(map[EntityID]string, len(actors))
	for _, r := range results {
		switch r.Verdict {
		case "preempted":
			out[r.ActorID] = "Things move too fast; you never get to finish what you started."
		case "altered":
			out[r.ActorID] = "You try, but it doesn't quite go the way you meant it to."
		default:
			out[r.ActorID] = fmt.Sprintf("You go ahead: %s", r.ActionText)
		}
	}
	return out
}
