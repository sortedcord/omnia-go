package sim

import (
	"context"
	"fmt"
	"strings"
	"time"

	"omnia.adityagupta.dev/pkg/actor"
	"omnia.adityagupta.dev/pkg/llm"
	"omnia.adityagupta.dev/pkg/memory"
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
	Personality   string
	Tick          int
	Timestamp     string
	Time          time.Time
	Stimulus      string
	Cognition     string // Raw internal reasoning tokens (empty for human player)
	Intent        string // What the actor attempted
	IsHuman       bool
	Thoughts      []actor.Thought
	Perceivable   []EntityRef
	KnownEntities map[string]string
	Mind          *memory.Mind
}

// DistillResult summarizes the mutations produced by the Thought Distiller for an actor.
type DistillResult struct {
	ActorID    EntityID           `json:"actor_id"`
	Tick       int                `json:"tick"`
	DossierOps []memory.DossierOp `json:"dossier_ops,omitempty"`
	Ops        []actor.ThoughtOp  `json:"ops,omitempty"`
	Applied    []actor.ThoughtOp  `json:"applied,omitempty"`
	Rejected   []string           `json:"rejected,omitempty"`
	Usage      llm.Usage          `json:"-"`
	Duration   time.Duration      `json:"-"`
	Err        error              `json:"-"`
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

// DistillerSchema returns the strict JSON schema for subconscious thought and dossier distillation.
func DistillerSchema(availableSubjectIDs []string, availableEntityIDs []string) *llm.Schema {
	subjSchema := llm.Str("Target subject ID")
	if len(availableSubjectIDs) > 0 {
		subjSchema = llm.Enum("Target subject ID", availableSubjectIDs...)
	}

	entitySchema := llm.Str("Entity ID from current perception")
	if len(availableEntityIDs) > 0 {
		entitySchema = llm.Enum("Entity ID from current perception", availableEntityIDs...)
	}

	opItem := llm.Obj("An atomic mutation on the actor's dossiers and mental reality.",
		llm.Prop("op", llm.Enum("Mutation operator: upsert_claim, retract_claim, set_impression, add_name, new_subject, bind, unbind.",
			"upsert_claim", "retract_claim", "set_impression", "add_name", "new_subject", "bind", "unbind")),
		llm.Prop("subject_id", subjSchema),
		llm.Prop("claim_id", llm.Nullable(llm.Str("Target claim ID (e.g. c-zack-1). Required for retract_claim; optional for upsert_claim; null for others."))),
		llm.Prop("kind", llm.Nullable(llm.Enum("Cognitive category: belief (conviction), intention (plan/goal), feeling (somatic/emotional sensation), suspicion (distrust/doubt).",
			"belief", "intention", "feeling", "suspicion"))),
		llm.Prop("content", llm.Nullable(llm.Str("First-person subjective thought text in character's authentic voice; null for retract_claim, unbind."))),
		llm.Prop("salience", llm.Nullable(llm.Num("Priority score between 0.05 and 1.00 for attentional focus; null if unchanged."))),
		llm.Prop("conviction", llm.Nullable(llm.Num("Certainty score between 0.00 and 1.00; null if unchanged."))),
		llm.Prop("name", llm.Nullable(llm.Str("Subjective alias or name for add_name or new_subject; null for others."))),
		llm.Prop("subject_kind", llm.Nullable(llm.Enum("Category of subject: person, place, object, group, idea; null for others.",
			"person", "place", "object", "group", "idea"))),
		llm.Prop("entity_id", llm.Nullable(entitySchema)),
		llm.Prop("bind_mode", llm.Nullable(llm.Enum("Binding mode: identity (this IS that) or anchor (located AT that); null for others.",
			"identity", "anchor"))),
	)

	return llm.Obj("Subconscious thought and dossier distillation ops.",
		llm.Prop("ops", llm.Arr("List of atomic operations to update the actor's mental reality (up to 4 ops).", opItem)),
	)
}

type distillWireResponse struct {
	Ops []memory.DossierOp `json:"ops"`
}

// Distill synthesizes subjective cognition and intent into atomic thought mutations.
func (d *LLMDistiller) Distill(ctx context.Context, in DistillInput) (DistillResult, error) {
	start := time.Now()
	res := DistillResult{
		ActorID: in.ActorID,
		Tick:    in.Tick,
	}

	systemPrompt := fmt.Sprintf(`You are the SUBCONSCIOUS THOUGHT DISTILLER for the character "%s".
Your role is to maintain their private, subjective dossiers and beliefs (self, people, places, intentions) according to the structured schema.

DISTILLATION RULES:
1. Strict In-World Subjectivity:
   - Discard all meta-reasoning: Never create thoughts about prompts, JSON schemas, formatting, token limits, or rules.
   - Filter raw stream-of-consciousness down into genuine in-character convictions, physical sensations, tactical plans, or suspicions.
   - The character's accessible private attributes reflect their conscious internal sensations and emotions (e.g. elevated heart rate, private nervousness) that naturally feed into their feelings and beliefs.
   - The character DOES NOT know if their action succeeded yet (the physics referee hasn't resolved it). Only distill what they felt, believed, and attempted.
2. Salience & Graph Discipline:
   - Use "upsert_claim" to add or update beliefs, intentions, feelings, and suspicions.
   - Intentions and feelings that are not about a specific external person should target your own self ID (%s).
   - Set higher salience (up to 1.00) for thoughts dominating immediate attention; lower for background awareness.
   - Emit between 0 and 4 high-quality ops per cycle. Do not spam.`,
		in.ActorName, in.ActorID)

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

	var subjectIDs []string
	if in.Mind != nil {
		for _, s := range in.Mind.SubjectsSnapshot() {
			subjectIDs = append(subjectIDs, s.ID)
		}
	}
	if len(subjectIDs) == 0 {
		subjectIDs = append(subjectIDs, string(in.ActorID))
	}

	var entityIDs []string
	for _, p := range in.Perceivable {
		entityIDs = append(entityIDs, string(p.ID))
	}

	req := llm.ChatRequest{
		Model: d.model,
		Messages: []llm.Message{
			{Role: llm.System, Content: systemPrompt},
			{Role: llm.User, Content: sb.String()},
		},
		ResponseFormat:   llm.StrictSchema("thought_distillation", "Mental graph mutations reflecting subjective experience.", DistillerSchema(subjectIDs, entityIDs)),
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

	// Post-decode enforcement: clamp salience and enforce max 4 ops
	if len(wire.Ops) > 4 {
		wire.Ops = wire.Ops[:4]
	}
	for i := range wire.Ops {
		if wire.Ops[i].Salience != nil {
			val := *wire.Ops[i].Salience
			if val < 0.05 {
				val = 0.05
			} else if val > 1.00 {
				val = 1.00
			}
			wire.Ops[i].Salience = &val
		}
	}

	res.DossierOps = wire.Ops

	// Back-compat mapping from DossierOp to ThoughtOp
	for _, dop := range wire.Ops {
		top := actor.ThoughtOp{
			Op:      actor.ThoughtOpType(dop.Op),
			ID:      dop.ClaimID,
			Kind:    actor.ThoughtKind(dop.Kind),
			Content: dop.Content,
		}
		if dop.Op == "upsert_claim" {
			top.Op = actor.OpAdd
			if dop.ClaimID != "" {
				top.Op = actor.OpUpdate
			}
		} else if dop.Op == "retract_claim" {
			top.Op = actor.OpDrop
		}
		if dop.Salience != nil {
			top.Salience = dop.Salience
		}
		if dop.SubjectID != "" {
			top.Links = []string{dop.SubjectID}
		}
		res.Ops = append(res.Ops, top)
	}

	return res, nil
}
