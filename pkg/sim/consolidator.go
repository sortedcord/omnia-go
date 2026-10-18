package sim

import (
	"context"
	"fmt"
	"strings"
	"time"

	"omnia.adityagupta.dev/pkg/llm"
	"omnia.adityagupta.dev/pkg/memory"
)

// ConsolidateInput packages the uncommitted turns and scene context for episode condensation.
type ConsolidateInput struct {
	ActorID      EntityID
	ActorName    string
	Personality  string
	SceneID      string
	LocationID   string
	FromSeq      int64
	ToSeq        int64
	Turns        []memory.Turn
	PriorBeats   []memory.Beat
	IsSceneClose bool // True if closing the episode; false if mid-scene rolling condensation
}

// ConsolidateResult captures the condensed beats and finalized episode metadata.
type ConsolidateResult struct {
	ActorID             EntityID
	FromSeq             int64
	ToSeq               int64
	Beats               []memory.Beat
	Title               string
	Summary             string
	Tone                string
	Importance          float64
	FormativeSubjectIDs []string
	Degraded            bool
	Duration            time.Duration
	Usage               llm.Usage
	Err                 error
}

// Consolidator defines the interface for condensing working buffer turns into episodic archive memories.
type Consolidator interface {
	Consolidate(ctx context.Context, in ConsolidateInput) (ConsolidateResult, error)
}

// LLMConsolidator implements Consolidator using structured chat completions.
type LLMConsolidator struct {
	client llm.Client
	model  string
}

// NewLLMConsolidator creates an LLM-backed episode consolidator.
func NewLLMConsolidator(client llm.Client, model string) *LLMConsolidator {
	if model == "" {
		model = "gemini-3.5-flash-lite"
	}
	return &LLMConsolidator{
		client: client,
		model:  model,
	}
}

// ConsolidatorSchema constructs the strict JSON schema for episode condensation.
func ConsolidatorSchema() *llm.Schema {
	beatItem := llm.Obj("A condensed chronological beat segment.",
		llm.Prop("text", llm.Str("First-person condensed segment (<= 60 words) capturing key actions, dialogue, and shifts.")),
		llm.Prop("subject_ids", llm.Arr("Entity or actor IDs that featured prominently in this beat.", llm.Str("Subject ID"))),
	)

	return llm.Obj("Condensed episodic archive memory.",
		llm.Prop("beats", llm.Arr("Chronological chain of condensed beats.", beatItem)),
		llm.Prop("title", llm.Str("Short evocative title for this episode (<= 12 words).")),
		llm.Prop("summary", llm.Str("First-person overview summary of what occurred and what it meant to you (<= 80 words).")),
		llm.Prop("tone", llm.Str("Overall emotional tone or atmosphere (e.g. 'mortified relief', 'awkward camaraderie').")),
		llm.Prop("importance", llm.Num("Formative importance score between 0.05 (mundane routine) and 1.00 (life-altering event).")),
		llm.Prop("formative_subject_ids", llm.Arr("IDs of subjects whose relationship or impression was strongly affected.", llm.Str("Subject ID"))),
	)
}

type consolidateWireResponse struct {
	Beats []struct {
		Text       string   `json:"text"`
		SubjectIDs []string `json:"subject_ids"`
	} `json:"beats"`
	Title               string   `json:"title"`
	Summary             string   `json:"summary"`
	Tone                string   `json:"tone"`
	Importance          float64  `json:"importance"`
	FormativeSubjectIDs []string `json:"formative_subject_ids"`
}

// Consolidate converts uncommitted turns into condensed beats and episode metadata.
func (c *LLMConsolidator) Consolidate(ctx context.Context, in ConsolidateInput) (ConsolidateResult, error) {
	start := time.Now()
	res := ConsolidateResult{
		ActorID: in.ActorID,
		FromSeq: in.FromSeq,
		ToSeq:   in.ToSeq,
	}

	if len(in.Turns) == 0 {
		return res, nil
	}

	systemPrompt := fmt.Sprintf(`You are the EPISODIC MEMORY CONSOLIDATOR for the character "%s".
Your role is to condense raw recent experiences from their working buffer into durable episodic archive memories.

CONSOLIDATION INVARIANTS:
1. Strict First-Person Subjectivity:
   - Condense ONLY what %s personally perceived, attempted, and felt.
   - Do NOT invent outside facts, objective referee outcomes, or hidden knowledge.
   - Write all summaries and beats in first-person ("I...", "We...").
2. Beat Compression:
   - Compress the turns into 1 to 3 chronological beats capturing major inflection points.
   - Keep beats concise (<= 60 words each).`,
		in.ActorName, in.ActorName)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ACTOR: %s | SCENE: %s | LOCATION: %s\n", in.ActorName, in.SceneID, in.LocationID))
	sb.WriteString(fmt.Sprintf("TURNS TO CONSOLIDATE (Seq %d to %d):\n\n", in.FromSeq, in.ToSeq))

	for _, t := range in.Turns {
		actionStr := t.Action.Action
		if len(t.Action.Targets) > 0 {
			actionStr = fmt.Sprintf("%s (targets: %v)", actionStr, t.Action.Targets)
		}
		sb.WriteString(fmt.Sprintf("[Turn Seq %d | Tick %d]\n- Perceived: %s\n- Did/Said: %s\n\n",
			t.Seq, t.Tick, t.Perception, actionStr))
	}

	sb.WriteString("Condense these turns into structured episodic beats and summary:")

	req := llm.ChatRequest{
		Model: c.model,
		Messages: []llm.Message{
			{Role: llm.System, Content: systemPrompt},
			{Role: llm.User, Content: sb.String()},
		},
		ResponseFormat:   llm.StrictSchema("episode_consolidation", "Condensed episodic archive memory.", ConsolidatorSchema()),
		ReasoningEffort:  llm.ReasoningLow,
		IncludeReasoning: true,
	}

	resp, err := c.client.ChatCompletion(ctx, req)
	if err != nil {
		res.Err = err
		return res, err
	}

	res.Duration = time.Since(start)
	res.Usage = resp.Usage

	var wire consolidateWireResponse
	if err := llm.DecodeJSON(resp.Content, resp.ReasoningContent, &wire); err != nil {
		res.Err = err
		return res, fmt.Errorf("consolidator: decode failed: %w (content: %q)", err, resp.Content)
	}

	for _, b := range wire.Beats {
		res.Beats = append(res.Beats, memory.Beat{
			FromSeq:    in.FromSeq,
			ToSeq:      in.ToSeq,
			Text:       strings.TrimSpace(b.Text),
			SubjectIDs: b.SubjectIDs,
		})
	}
	res.Title = strings.TrimSpace(wire.Title)
	res.Summary = strings.TrimSpace(wire.Summary)
	res.Tone = strings.TrimSpace(wire.Tone)
	res.Importance = wire.Importance
	if res.Importance < 0.05 {
		res.Importance = 0.05
	} else if res.Importance > 1.00 {
		res.Importance = 1.00
	}
	res.FormativeSubjectIDs = wire.FormativeSubjectIDs

	return res, nil
}
