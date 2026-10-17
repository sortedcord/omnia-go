package actor

import (
	"encoding/json"
	"fmt"
	"strings"
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
	Type    ActionType `json:"action_type"`
	Action  string     `json:"action"`
	Targets []string   `json:"targets,omitempty"` // Names of intended addressees or interactive targets
}

// Profile defines the immutable or slow-evolving identity of the actor.
type Profile struct {
	ID          string
	Name        string
	Personality string
	Directives  []string
	Background  string
}

// Attribute represents a dynamic somatic, psychological, or situational property of an actor.
type Attribute struct {
	Value   string   `json:"value"`
	Allowed []string `json:"allowed,omitempty"` // Non-empty indicates an enum
	Core    bool     `json:"core,omitempty"`    // Core attributes cannot be removed
}

// Attributes maps attribute keys to their values and constraints.
type Attributes map[string]Attribute

// NewAttributes initializes default core attributes (mood and arousal).
func NewAttributes() Attributes {
	attrs := make(Attributes)
	attrs["mood"] = Attribute{
		Value: "Neutral",
		Core:  true,
	}
	attrs["arousal"] = Attribute{
		Value:   "normal",
		Allowed: []string{"low", "normal", "high"},
		Core:    true,
	}
	return attrs
}

// Get retrieves the current string value of an attribute.
func (a Attributes) Get(key string) (string, bool) {
	attr, ok := a[key]
	if !ok {
		return "", false
	}
	return attr.Value, true
}

// Set mutates an attribute value, enforcing enum constraints if configured.
func (a Attributes) Set(key, value string) error {
	attr, exists := a[key]
	if exists && len(attr.Allowed) > 0 {
		valid := false
		for _, allowed := range attr.Allowed {
			if strings.EqualFold(allowed, value) {
				valid = true
				value = allowed
				break
			}
		}
		if !valid {
			return fmt.Errorf("attributes: invalid value %q for enum attribute %q (allowed: %v)", value, key, attr.Allowed)
		}
	}
	if exists {
		attr.Value = value
		a[key] = attr
	} else {
		a[key] = Attribute{Value: value}
	}
	return nil
}

// Remove deletes an attribute if it is not marked as Core.
func (a Attributes) Remove(key string) error {
	attr, ok := a[key]
	if !ok {
		return nil
	}
	if attr.Core {
		return fmt.Errorf("attributes: cannot remove core attribute %q", key)
	}
	delete(a, key)
	return nil
}

// Copy returns an isolated deep copy of the attributes map.
func (a Attributes) Copy() Attributes {
	out := make(Attributes, len(a))
	for k, v := range a {
		allowedCopy := append([]string(nil), v.Allowed...)
		out[k] = Attribute{
			Value:   v.Value,
			Allowed: allowedCopy,
			Core:    v.Core,
		}
	}
	return out
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
