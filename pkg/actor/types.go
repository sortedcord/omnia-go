package actor

import (
	"encoding/json"
	"fmt"
	"time"

	"omnia.adityagupta.dev/pkg/llm"
)

// ActionType represents manifest external actions taken by an actor.
// Internal cognition is handled natively via reasoning tokens.
type ActionType int

const (
	Speak ActionType = iota
	Act
	Wait
)

func (a ActionType) String() string {
	switch a {
	case Speak:
		return "speak"
	case Act:
		return "act"
	case Wait:
		return "wait"
	default:
		return fmt.Sprintf("ActionType(%d)", a)
	}
}

func (a ActionType) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

func (a *ActionType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "speak":
		*a = Speak
	case "act":
		*a = Act
	case "wait":
		*a = Wait
	default:
		return fmt.Errorf("unknown action type: %q", s)
	}
	return nil
}

// Action is the structured external manifestation emitted by an actor into the world.
type Action struct {
	Type        ActionType `json:"action_type"`
	Description string     `json:"action"`
}

// Profile defines the immutable or slow-evolving identity of the actor.
type Profile struct {
	ID          string
	Name        string
	Role        string
	Personality string
	Directives  []string
	Background  string
}

// Status represents the actor's internal somatic and emotional state.
// Spatial location and inventory are derived from the simulation's entity tree.
type Status struct {
	Mood   string
	Vitals map[string]string // e.g. "Oxygen Reserve": "82%", "Weariness": "Moderate"
}

// SensoryEvent represents an external event as subjectively perceived by this actor.
type SensoryEvent struct {
	Tick        int                `json:"tick"`
	Timestamp   string             `json:"timestamp"`   // e.g. "Cycle 104, 03:16 UTC"
	Description string             `json:"description"` // What the actor's senses register
	Relevant    map[string]float64 `json:"-"`           // Entity IDs currently in perception/focus -> weight
}

// String formats the sensory event for prompt injection.
func (e SensoryEvent) String() string {
	if e.Timestamp != "" {
		return fmt.Sprintf("[SIMULATION SENSORY EVENT: %s]\n%s", e.Timestamp, e.Description)
	}
	return fmt.Sprintf("[SIMULATION SENSORY EVENT]\n%s", e.Description)
}

// Reaction contains the actor's full response to a sensory event:
// both private internal thoughts (cognition) and public physical actions.
type Reaction struct {
	Cognition string        // Private internal stream of consciousness (from reasoning tokens)
	Action    Action        // Public manifest action (speak, act, or wait)
	Usage     llm.Usage     // Token metrics
	Duration  time.Duration // Time taken for deliberation and response
}
