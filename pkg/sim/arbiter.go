package sim

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"omnia.adityagupta.dev/pkg/actor"
	"omnia.adityagupta.dev/pkg/llm"
)

// EffectType represents a valid mutation operator on the world state.
type EffectType string

const (
	EffectMove            EffectType = "move"             // Reparents an entity: entity_id, parent_id, relation, placement
	EffectSetCondition    EffectType = "set_condition"    // Updates space condition: entity_id, key, value
	EffectSetPortal       EffectType = "set_portal"       // Updates portal state: entity_id, closed, locked
	EffectSetAttribute    EffectType = "set_attribute"    // Updates actor attribute: entity_id, key, value
	EffectRemoveAttribute EffectType = "remove_attribute" // Removes non-core attribute: entity_id, key
	EffectSetOpen         EffectType = "set_open"         // Updates container openness: entity_id, open
	EffectReveal          EffectType = "reveal"           // Clears concealment on an entity: entity_id
)

// Effect is an atomic world-state mutation commanded by the Arbiter.
type Effect struct {
	Type       EffectType `json:"type"`
	EntityID   EntityID   `json:"entity_id,omitempty"`
	ParentID   EntityID   `json:"parent_id,omitempty"`
	Relation   Relation   `json:"relation,omitempty"`
	Placement  string     `json:"placement,omitempty"`
	Key        string     `json:"key,omitempty"`
	Value      string     `json:"value,omitempty"`
	Visibility Visibility `json:"visibility,omitempty"` // "public" or "private"
	Accessors  []EntityID `json:"accessors,omitempty"`  // Permitted observers when private
	Closed     *bool      `json:"closed,omitempty"`
	Locked     *bool      `json:"locked,omitempty"`
	Open       *bool      `json:"open,omitempty"`
}

// ActorIntent packages an actor's declared intent for scene adjudication.
type ActorIntent struct {
	ActorID     EntityID          `json:"actor_id"`
	ActorName   string            `json:"actor_name"`
	ActionType  actor.ActionType  `json:"action_type"`
	ActionText  string            `json:"action_text"`
	Targets     []EntityID        `json:"targets,omitempty"`
	Spontaneity float64           `json:"spontaneity"`
	Luck        float64           `json:"luck"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

// SceneAdjudication packages all concurrent actor intents and scene context for the Arbiter.
type SceneAdjudication struct {
	SceneID         SceneID
	Tick            int
	Timestamp       string
	Intents         []ActorIntent
	WorldState      string   // Objective, unfiltered description of the scene's spaces and entities
	SceneEvents     []string // Chronological recent occurrences in this scene
	GlobalEvents    []string // Digest of recent events across the wider simulation
	TickDurationSec int
}

// IntentVerdict provides the referee's judgment on an individual actor's intent.
type IntentVerdict struct {
	ActorID         EntityID `json:"actor_id"`
	Verdict         string   `json:"verdict"` // as_intended | altered | preempted
	Outcome         string   `json:"outcome"` // success | partial | failure | mishap | interrupted
	RemainingIntent string   `json:"remaining_intent,omitempty"`
}

// SceneRuling is the Arbiter's authoritative resolution of all concurrent actions in a scene.
type SceneRuling struct {
	SceneID         SceneID         `json:"scene_id"`
	Verdicts        []IntentVerdict `json:"verdicts"`
	ExecutionOrder  []EntityID      `json:"execution_order"` // Order in which actions physically materialized
	Resolution      string          `json:"resolution"`      // Objective third-person canonical truth of the scene
	Effects         []Effect        `json:"effects,omitempty"`
	AppliedEffects  []Effect        `json:"-"`
	RejectedEffects []string        `json:"-"`
	Usage           llm.Usage       `json:"-"`
	Duration        time.Duration   `json:"-"`
}

// Arbiter defines the contract for refereeing all concurrent actor intents in a scene.
type Arbiter interface {
	ResolveScene(ctx context.Context, adj SceneAdjudication) (*SceneRuling, error)
}

// LLMArbiter implements Arbiter using chat completions.
type LLMArbiter struct {
	client llm.Client
	model  string
}

// NewLLMArbiter creates an Arbiter backed by an LLM client.
func NewLLMArbiter(client llm.Client, model string) *LLMArbiter {
	if model == "" {
		model = "gemini-3.5-flash-lite"
	}
	return &LLMArbiter{
		client: client,
		model:  model,
	}
}

// ResolveScene refereees all concurrent actor intents in a scene and returns the authoritative ruling.
func (a *LLMArbiter) ResolveScene(ctx context.Context, adj SceneAdjudication) (*SceneRuling, error) {
	start := time.Now()

	systemPrompt := fmt.Sprintf(`You are THE ARBITER: an impartial, physically realistic simulation referee and physics engine.
Actors in this scene submitted their actions SIMULTANEOUSLY. You determine what ACTUALLY HAPPENS.

OUTPUT REQUIREMENT:
You MUST respond ONLY with a valid JSON object matching this schema:
{
  "execution_order": ["<actor_id_1>", "<actor_id_2>"],
  "verdicts": [
    {
      "actor_id": "<exact_actor_id>",
      "verdict": "as_intended" | "altered" | "preempted",
      "outcome": "success" | "partial" | "failure" | "mishap" | "interrupted",
      "remaining_intent": "<any uncompleted portion of the intent, or empty string>"
    }
  ],
  "resolution": "<concise objective third-person ground truth of everything that physically happened in the scene>",
  "effects": [
    {
      "type": "move" | "set_condition" | "set_portal" | "set_attribute" | "remove_attribute" | "set_open" | "reveal",
      "entity_id": "<exact entity ID from world state>",
      "parent_id": "<exact target parent entity ID for move>",
      "relation": "in" | "on" | "under" | "behind" | "worn" | "held" | "seated",
      "placement": "<optional placement description>",
      "key": "<condition or attribute name>",
      "value": "<new value>",
      "visibility": "public" | "private",
      "accessors": ["<entity_id_1>"],
      "closed": true | false,
      "locked": true | false,
      "open": true | false
    }
  ]
}

REFEREE RULES:
1. Simultaneity & Precedence:
   - Actors acted concurrently this tick (%ds window).
   - Use Spontaneity scores and Luck rolls as guidance for who acts first or wins contests.
   - If two actors collide (e.g. both reaching for the same item), the higher spontaneity/reflexes and physical position wins; the second actor's action is "altered" or "preempted".
   - If an actor was interrupted before completing their action, mark verdict as "altered" or "preempted" with outcome "interrupted", and store unfinished action in "remaining_intent".
2. Verdicts:
   - "as_intended": Action proceeded without major disruption.
   - "altered": Action was forced to adapt, deflected, or partially interrupted by another actor's action or environment.
   - "preempted": The situation changed so fast the action never got off the ground.
3. Outcome Calibration:
   - "success": Attempt succeeded as planned.
   - "partial": Progress made, but bounded by time or partial interference.
   - "failure": Failed cleanly without disaster.
   - "mishap": Physical blunder, slip, or fumble (especially with low Luck <= 0.20 or stressful conditions).
   - "interrupted": Cut short mid-stride.
4. Objective Resolution Only:
   - Describe purely the objective physical ground truth of the scene in past tense.
   - Do NOT write subjective second-person text (the Broadcaster handles subjective perceptions).
5. Effects Whitelist:
   - "move": Reparents entity.
   - "set_condition": Updates room condition (empty value deletes).
   - "set_portal": Sets closed/locked on portals.
   - "set_attribute": Updates an attribute on ANY entity (actor, prop, vehicle, door). Set "visibility": "public" (clears accessors) or "private" with specific "accessors". An attribute with "private" and empty "accessors": [] is completely hidden from everyone (even the entity itself!), known only to you (the Arbiter) to realistically influence outcomes and alter actions!
   - "remove_attribute": Removes a non-core attribute. Core attributes (mood, arousal) cannot be removed.
   - "set_open": Sets open=true/false on a container.
   - "reveal": Sets concealed=false.
   - NEVER invent new entity IDs. Only use IDs from the WORLD STATE.`,
		adj.TickDurationSec)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("SCENE: %s | TICK: %d | TIME: %s | DURATION: %ds\n\n",
		adj.SceneID, adj.Tick, adj.Timestamp, adj.TickDurationSec))

	sb.WriteString("CONCURRENT ACTOR INTENTS (Submitted simultaneously this tick):\n")
	for _, in := range adj.Intents {
		targetsStr := "none"
		if len(in.Targets) > 0 {
			var tStrs []string
			for _, t := range in.Targets {
				tStrs = append(tStrs, string(t))
			}
			targetsStr = strings.Join(tStrs, ", ")
		}

		attrsStr := ""
		if len(in.Attributes) > 0 {
			var aStrs []string
			for k, v := range in.Attributes {
				aStrs = append(aStrs, fmt.Sprintf("%s=%s", k, v))
			}
			attrsStr = fmt.Sprintf(" | attrs: [%s]", strings.Join(aStrs, ", "))
		}

		sb.WriteString(fmt.Sprintf("- Actor: %s (ID: %s) | Type: %s | Spontaneity: %.2f | Luck: %.2f | Targets: [%s]%s\n  Intent: %q\n",
			in.ActorName, in.ActorID, in.ActionType, in.Spontaneity, in.Luck, targetsStr, attrsStr, in.ActionText))
	}

	sb.WriteString("\nOBJECTIVE WORLD STATE (Ground Truth of Scene's Spaces):\n")
	sb.WriteString(adj.WorldState)

	if len(adj.SceneEvents) > 0 {
		sb.WriteString("\n\nRECENT EVENTS IN THIS SCENE:\n")
		for _, ev := range adj.SceneEvents {
			sb.WriteString(fmt.Sprintf("- %s\n", ev))
		}
	}

	if len(adj.GlobalEvents) > 0 {
		sb.WriteString("\nRECENT GLOBAL EVENTS (Across Simulation):\n")
		for _, ev := range adj.GlobalEvents {
			sb.WriteString(fmt.Sprintf("- %s\n", ev))
		}
	}

	sb.WriteString("\nAdjudicate the scene:")

	req := llm.ChatRequest{
		Model: a.model,
		Messages: []llm.Message{
			{Role: llm.System, Content: systemPrompt},
			{Role: llm.User, Content: sb.String()},
		},
		ResponseFormat:   "json_object",
		ReasoningEffort:  llm.ReasoningMedium,
		IncludeReasoning: true,
	}

	resp, err := a.client.ChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("arbiter: chat completion failed for scene %q: %w", adj.SceneID, err)
	}

	var ruling SceneRuling
	if err := llm.DecodeJSON(resp.Content, resp.ReasoningContent, &ruling); err != nil {
		return nil, fmt.Errorf("arbiter: failed to decode scene ruling JSON: %w (content: %q)", err, resp.Content)
	}

	ruling.SceneID = adj.SceneID
	ruling.Duration = time.Since(start)
	ruling.Usage = resp.Usage

	return &ruling, nil
}

// orderEffects applies effects in the arbiter's execution order. Effects are attributed
// to the actor whose entity they touch (or who carries it); unattributed effects keep
// their relative order after the attributed ones. The sort is stable.
func orderEffects(r *SceneRuling) {
	if len(r.ExecutionOrder) == 0 || len(r.Effects) < 2 {
		return
	}
	rank := make(map[EntityID]int, len(r.ExecutionOrder))
	for i, id := range r.ExecutionOrder {
		if _, seen := rank[id]; !seen {
			rank[id] = i
		}
	}
	position := func(e Effect) int {
		for _, id := range []EntityID{e.EntityID, e.ParentID} {
			if p, ok := rank[id]; ok {
				return p
			}
		}
		return len(rank)
	}
	sort.SliceStable(r.Effects, func(i, j int) bool {
		return position(r.Effects[i]) < position(r.Effects[j])
	})
}
