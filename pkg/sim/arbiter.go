package sim

import (
	"context"
	"fmt"
	"strings"
	"time"

	"omnia.adityagupta.dev/pkg/llm"
)

// OutcomeType represents the Arbiter's verdict on an attempted action.
type OutcomeType string

const (
	OutcomeSuccess   OutcomeType = "success"     // Action succeeded fully as attempted
	OutcomePartial   OutcomeType = "partial"     // Action made progress but was constrained or incomplete
	OutcomeFailure   OutcomeType = "failure"     // Action failed cleanly without disaster
	OutcomeMishap    OutcomeType = "mishap"      // Action failed with a blunder, fumble, or self-inflicted complication
	OutcomeInterrupt OutcomeType = "interrupted" // Action was cut short mid-stride by an obstacle or external event
)

// EffectType represents a valid mutation operator on the world state.
type EffectType string

const (
	EffectMove         EffectType = "move"          // Reparents an entity: entity_id, parent_id, relation, placement
	EffectSetCondition EffectType = "set_condition" // Updates space condition: entity_id, key, value
	EffectSetPortal    EffectType = "set_portal"    // Updates portal state: entity_id, closed, locked
	EffectSetVital     EffectType = "set_vital"     // Updates actor vitals: entity_id, key, value
	EffectSetOpen      EffectType = "set_open"      // Updates container openness: entity_id, open
	EffectReveal       EffectType = "reveal"        // Clears concealment on an entity: entity_id
)

// Effect is an atomic world-state mutation commanded by the Arbiter.
type Effect struct {
	Type      EffectType `json:"type"`
	EntityID  EntityID   `json:"entity_id,omitempty"`
	ParentID  EntityID   `json:"parent_id,omitempty"`
	Relation  Relation   `json:"relation,omitempty"`
	Placement string     `json:"placement,omitempty"`
	Key       string     `json:"key,omitempty"`
	Value     string     `json:"value,omitempty"`
	Closed    *bool      `json:"closed,omitempty"`
	Locked    *bool      `json:"locked,omitempty"`
	Open      *bool      `json:"open,omitempty"`
}

// Adjudication packages everything the Arbiter needs to referee an actor's declared intent.
type Adjudication struct {
	Tick            int
	Timestamp       string
	ActorID         EntityID
	ActorName       string
	ActorVitals     map[string]string
	ActorMood       string
	Intent          string
	WorldState      string   // Objective, unfiltered description of the room and all entities
	RecentEvents    []string // Chronological recent occurrences in this room
	Luck            float64  // Seeded random roll between 0.00 (terrible) and 1.00 (flawless)
	TickDurationSec int
}

// Ruling is the Arbiter's authoritative resolution of an attempted action.
type Ruling struct {
	Outcome            OutcomeType   `json:"outcome"`
	Resolution         string        `json:"resolution"`          // Objective third-person canonical truth
	ActorPerception    string        `json:"actor_perception"`    // Second-person subjective experience for the acting actor
	ObserverPerception string        `json:"observer_perception"` // Third-person sensory description for co-present onlookers
	RemainingIntent    string        `json:"remaining_intent"`    // Unfinished trajectory to carry forward to the next tick
	Effects            []Effect      `json:"effects,omitempty"`   // Requested world-state mutations
	Luck               float64       `json:"-"`
	AppliedEffects     []Effect      `json:"-"`
	RejectedEffects    []string      `json:"-"`
	Usage              llm.Usage     `json:"-"`
	Duration           time.Duration `json:"-"`
}

// Arbiter defines the contract for resolving subjective intent into objective reality.
type Arbiter interface {
	Resolve(ctx context.Context, adj Adjudication) (*Ruling, error)
}

// LLMArbiter implements Arbiter via chat completions.
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

// Resolve adjudicates an attempted action and returns the authoritative ruling.
func (a *LLMArbiter) Resolve(ctx context.Context, adj Adjudication) (*Ruling, error) {
	systemPrompt := fmt.Sprintf(`You are THE ARBITER: an impartial, physically realistic simulation referee and physics engine.
Actors in the simulation only declare their INTENT (what they attempt to do). You determine what ACTUALLY HAPPENS.

CORE REFEREE RULES:
1. Pacing & Time: Each simulation tick represents %d seconds. An actor can only accomplish what physically fits within this window. If their intent describes a multi-step journey (e.g. "Walk to the kitchen and make coffee"), resolve only the FIRST %d seconds of progress, and put the rest into "remaining_intent".
2. No God-Moding: An actor cannot decree success against hazardous or mechanical obstacles. Evaluate their physical state (vitals, fatigue, mood), ambient hazards (alarms, fires, depressurization), and the provided Luck roll.
3. Luck Calibration (0.00 to 1.00):
   - High (>= 0.80): Fortune favors the attempt; close calls break the actor's way.
   - Normal (0.25 to 0.79): Success depends strictly on physical plausibility, gear, and environmental difficulty.
   - Low (<= 0.24): Complications, slips, or equipment snags are likely under stressful conditions.
   - Critical Low (<= 0.10): If the attempt is hazardous or rushed, a genuine mishap or fumble occurs (e.g. dropping a tool, stumbling).
   - Trivial, safe actions (e.g. looking around, standing up) always succeed regardless of luck.
4. Output Schema: You MUST respond ONLY with a valid JSON object matching this schema:
{
  "outcome": "success" | "partial" | "failure" | "mishap" | "interrupted",
  "resolution": "<brief objective third-person ground truth of what physically occurred in past tense>",
  "actor_perception": "<second-person sensory description of what the actor felt, heard, saw, or experienced>",
  "observer_perception": "<third-person description of what co-present onlookers in the room witness, or empty string if silent/invisible>",
  "remaining_intent": "<any uncompleted portion of the actor's declared intent, or empty string if fully resolved>",
  "effects": [
    {
      "type": "move" | "set_condition" | "set_portal" | "set_vital" | "set_open" | "reveal",
      "entity_id": "<exact entity ID from world state>",
      "parent_id": "<exact target parent entity ID for move>",
      "relation": "in" | "on" | "under" | "behind" | "worn" | "held" | "seated",
      "placement": "<optional placement description>",
      "key": "<condition or vital name>",
      "value": "<new value>",
      "closed": true | false,
      "locked": true | false,
      "open": true | false
    }
  ]
}

EFFECTS RULES:
- Only emit effects when something actually changes state.
- "move": Moves an entity (item or actor) to a new parent. Use relation "held" (in hands), "worn" (on body), "on" (floor/surface), "in" (container/pocket/cabin).
- "set_condition": Changes an environmental condition on the room space. An empty value deletes the condition.
- "set_portal": Sets "closed" or "locked" on a portal (door/hatch).
- "set_vital": Changes an actor's vital stat (e.g. "Heart Rate": "Racing").
- "set_open": Opens or closes an enterable or container space (e.g. vehicle trunk, toolbox).
- "reveal": Sets concealed=false on an entity that has been uncovered or searched.
- CRITICAL: You must ONLY reference exact entity IDs listed in the WORLD STATE. Never invent new entity IDs.`,
		adj.TickDurationSec, adj.TickDurationSec)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("TICK: %d | TIME: %s | DURATION: %ds\n", adj.Tick, adj.Timestamp, adj.TickDurationSec))
	sb.WriteString(fmt.Sprintf("ACTOR: %s (ID: %s)\n", adj.ActorName, adj.ActorID))
	sb.WriteString(fmt.Sprintf("EMOTIONAL STATE: %s\n", adj.ActorMood))
	if len(adj.ActorVitals) > 0 {
		sb.WriteString("VITALS:\n")
		for k, v := range adj.ActorVitals {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", k, v))
		}
	}
	sb.WriteString(fmt.Sprintf("LUCK ROLL: %.2f\n\n", adj.Luck))
	sb.WriteString(fmt.Sprintf("ACTOR'S DECLARED INTENT:\n%q\n\n", adj.Intent))
	sb.WriteString("OBJECTIVE WORLD STATE (Ground Truth of Current Area):\n")
	sb.WriteString(adj.WorldState)

	if len(adj.RecentEvents) > 0 {
		sb.WriteString("\n\nRECENT EVENTS IN THIS AREA:\n")
		for _, ev := range adj.RecentEvents {
			sb.WriteString(fmt.Sprintf("- %s\n", ev))
		}
	}

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

	start := time.Now()
	resp, err := a.client.ChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("arbiter: chat completion failed: %w", err)
	}

	var ruling Ruling
	if err := llm.DecodeJSON(resp.Content, resp.ReasoningContent, &ruling); err != nil {
		return nil, fmt.Errorf("arbiter: failed to decode ruling JSON: %w (content: %q)", err, resp.Content)
	}

	ruling.Duration = time.Since(start)
	ruling.Usage = resp.Usage
	ruling.Luck = adj.Luck

	// Validate outcome
	switch ruling.Outcome {
	case OutcomeSuccess, OutcomePartial, OutcomeFailure, OutcomeMishap, OutcomeInterrupt:
		// Valid
	default:
		ruling.Outcome = OutcomePartial
	}

	return &ruling, nil
}
