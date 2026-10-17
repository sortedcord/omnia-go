package sim

import (
	"context"
	"fmt"
	"strings"
	"time"

	"omnia.adityagupta.dev/pkg/actor"
	"omnia.adityagupta.dev/pkg/llm"
)

// EntityRef represents a known entity with its perceived name and spatial relationship.
type EntityRef struct {
	ID       EntityID `json:"id"`
	Name     string   `json:"name"`
	Relation string   `json:"relation,omitempty"`
}

// DistillInput contains the subjective turn inputs needed to distill thought mutations.
type DistillInput struct {
	ActorID       EntityID
	ActorName     string
	ActorRole     string
	Personality   string
	Tick          int
	Timestamp     string
	Stimulus      string
	Cognition     string // Raw internal reasoning tokens (empty for human player)
	Intent        string // What the actor attempted
	IsHuman       bool
	Thoughts      []actor.Thought
	Perceivable   []EntityRef
	KnownEntities map[string]string
}

// DistillResult summarizes the mutations produced by the Thought Distiller for an actor.
type DistillResult struct {
	ActorID  EntityID          `json:"actor_id"`
	Tick     int               `json:"tick"`
	Ops      []actor.ThoughtOp `json:"ops,omitempty"`
	Applied  []actor.ThoughtOp `json:"applied,omitempty"`
	Rejected []string          `json:"rejected,omitempty"`
	Usage    llm.Usage         `json:"-"`
	Duration time.Duration     `json:"-"`
	Err      error             `json:"-"`
}

// ThoughtDistiller analyzes an actor's cognitive stream and updates their thought graph.
type ThoughtDistiller interface {
	Distill(ctx context.Context, input DistillInput) (DistillResult, error)
}

// LLMDistiller implements ThoughtDistiller using chat completions.
type LLMDistiller struct {
	client llm.Client
	model  string
}

// NewLLMDistiller creates an LLM-backed thought distiller.
func NewLLMDistiller(client llm.Client, model string) *LLMDistiller {
	if model == "" {
		model = "gemini-3.5-flash-lite"
	}
	return &LLMDistiller{
		client: client,
		model:  model,
	}
}

type distillWireResponse struct {
	Ops []actor.ThoughtOp `json:"ops"`
}

// Distill synthesizes subjective cognition and intent into atomic thought mutations.
func (d *LLMDistiller) Distill(ctx context.Context, in DistillInput) (DistillResult, error) {
	start := time.Now()
	res := DistillResult{
		ActorID: in.ActorID,
		Tick:    in.Tick,
	}

	systemPrompt := fmt.Sprintf(`You are the SUBCONSCIOUS THOUGHT DISTILLER for the character "%s" (%s).
Your role is to maintain their private, subjective thought graph (beliefs, intentions, feelings, suspicions).

OUTPUT REQUIREMENT:
You MUST respond ONLY with a valid JSON object matching this schema:
{
  "ops": [
    {
      "op": "add" | "update" | "link" | "unlink" | "set_salience" | "drop",
      "id": "<th-ID, required for update/link/unlink/set_salience/drop>",
      "kind": "belief" | "intention" | "feeling" | "suspicion",
      "content": "<first-person thought prose>",
      "links": ["<exact_entity_id_1>", "<exact_entity_id_2>"],
      "salience": 0.75
    }
  ]
}

DISTILLATION RULES:
1. Strict In-World Subjectivity:
   - Discard all meta-reasoning: Never create thoughts about prompts, JSON schemas, formatting, token limits, or rules.
   - Filter raw stream-of-consciousness down into genuine in-character convictions, physical sensations, tactical plans, or suspicions.
   - The character DOES NOT know if their action succeeded yet (the physics referee hasn't resolved it). Only distill what they felt, believed, and attempted.
2. Thought Categories:
   - "intention": A personal goal or plan (e.g. "I must reach the breaker box to save the canopy").
   - "belief": A subjective factual conviction about reality (e.g. "The secondary greenhouse viewport is failing"). Note: beliefs can be flawed or mistaken!
   - "suspicion": Distrust or doubt about a person, machine, or event (e.g. "DX-9 didn't short out by accident").
   - "feeling": Somatic or emotional sensations (e.g. "My knee is throbbing terribly from the impact", "Panic is setting in").
3. Salience & Graph Discipline:
   - Salience is 0.05 to 1.00. Set higher salience for thoughts dominating current attention.
   - Favor updating existing thoughts or adjusting their salience over creating duplicate thoughts.
   - When an intention is completed, obsolete, or abandoned, use "drop" to forget it.
   - Produce between 0 and 4 high-quality ops per cycle. Do not spam.
4. Entity Linking:
   - You may ONLY link to entity IDs explicitly provided in PERCEIVABLE ENTITIES or KNOWN ENTITIES. Never invent IDs.`,
		in.ActorName, in.ActorRole)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CHARACTER: %s | PERSONALITY: %s\n", in.ActorName, in.Personality))
	sb.WriteString(fmt.Sprintf("TICK: %d | TIME: %s\n\n", in.Tick, in.Timestamp))

	sb.WriteString("CURRENT EXISTING THOUGHTS IN MEMORY:\n")
	if len(in.Thoughts) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for _, t := range in.Thoughts {
			linksText := ""
			if len(t.Links) > 0 {
				linksText = fmt.Sprintf(" [links: %s]", strings.Join(t.Links, ", "))
			}
			sb.WriteString(fmt.Sprintf("- [%s] (%s, salience: %.2f) %s%s\n", t.ID, t.Kind, t.Salience, t.Content, linksText))
		}
	}

	sb.WriteString("\nPERCEIVABLE ENTITIES (Available for linking):\n")
	for _, p := range in.Perceivable {
		rel := ""
		if p.Relation != "" {
			rel = fmt.Sprintf(" (%s)", p.Relation)
		}
		sb.WriteString(fmt.Sprintf("- %s [ID: %s]%s\n", p.Name, p.ID, rel))
	}

	sb.WriteString(fmt.Sprintf("\nTHIS TURN'S SENSORY INGRESS:\n%s\n\n", in.Stimulus))
	if in.Cognition != "" {
		sb.WriteString(fmt.Sprintf("ACTOR'S RAW INTERNAL MONOLOGUE:\n%s\n\n", in.Cognition))
	}
	sb.WriteString(fmt.Sprintf("ACTOR'S DECLARED INTENT:\n%q\n\n", in.Intent))
	sb.WriteString("Distill cognitive updates into structured thought ops:")

	req := llm.ChatRequest{
		Model: d.model,
		Messages: []llm.Message{
			{Role: llm.System, Content: systemPrompt},
			{Role: llm.User, Content: sb.String()},
		},
		ResponseFormat:   "json_object",
		ReasoningEffort:  llm.ReasoningLow, // Low reasoning effort is fast and concise
		IncludeReasoning: true,
	}

	resp, err := d.client.ChatCompletion(ctx, req)
	if err != nil {
		res.Err = err
		return res, err
	}

	res.Duration = time.Since(start)
	res.Usage = resp.Usage

	var wire distillWireResponse
	if err := llm.DecodeJSON(resp.Content, resp.ReasoningContent, &wire); err != nil {
		res.Err = err
		return res, fmt.Errorf("distiller: decode failed: %w (content: %q)", err, resp.Content)
	}

	res.Ops = wire.Ops
	return res, nil
}
