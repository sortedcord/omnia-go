package actor

import "fmt"

// ThoughtKind classifies the nature of a subjective thought.
type ThoughtKind string

const (
	ThoughtBelief    ThoughtKind = "belief"    // Subjective conviction about the world, which may be mistaken
	ThoughtIntention ThoughtKind = "intention" // An ongoing goal or personal plan of action
	ThoughtFeeling   ThoughtKind = "feeling"   // Visceral emotional or physical sensation (fear, fatigue, relief)
	ThoughtSuspicion ThoughtKind = "suspicion" // Doubt or mistrust regarding an entity, event, or explanation
)

// Valid returns true if the kind is one of the recognized categories.
func (k ThoughtKind) Valid() bool {
	switch k {
	case ThoughtBelief, ThoughtIntention, ThoughtFeeling, ThoughtSuspicion:
		return true
	default:
		return false
	}
}

// Thought is a mutable, subjective atomic unit of cognition belonging to an actor.
// It can be tied to any number of entities (people, props, vehicles, rooms) by ID.
type Thought struct {
	ID       string      `json:"id"`
	Kind     ThoughtKind `json:"kind"`
	Content  string      `json:"content"`         // First-person prose from the actor's perspective
	Links    []string    `json:"links,omitempty"` // Referenced entity IDs (e.g. drone-dx9, breaker-box)
	Salience float64     `json:"salience"`        // Priority (0.0 to 1.0); higher values dominate attention
	Formed   int         `json:"formed_tick"`     // Simulation tick when this thought originated
	Updated  int         `json:"updated_tick"`    // Simulation tick when this thought was last reinforced or altered
}

// String provides a compact human-readable representation of a thought.
func (t Thought) String() string {
	return fmt.Sprintf("[%s:%s (%.2f)] %s", t.ID, t.Kind, t.Salience, t.Content)
}

// ThoughtOpType is the mutation command emitted by the Thought Distiller.
type ThoughtOpType string

const (
	OpAdd         ThoughtOpType = "add"          // Creates a new thought: requires kind, content; optional links, salience
	OpUpdate      ThoughtOpType = "update"       // Modifies content and/or kind of an existing thought: requires id
	OpLink        ThoughtOpType = "link"         // Appends entity IDs to a thought's links: requires id, links
	OpUnlink      ThoughtOpType = "unlink"       // Removes entity IDs from a thought's links: requires id, links
	OpSetSalience ThoughtOpType = "set_salience" // Adjusts the salience score of an existing thought: requires id, salience
	OpDrop        ThoughtOpType = "drop"         // Forgets or abandons a thought entirely: requires id
)

// ThoughtOp represents a structured atomic mutation on an actor's thought graph.
type ThoughtOp struct {
	Op       ThoughtOpType `json:"op"`
	ID       string        `json:"id,omitempty"`       // Required for update, link, unlink, set_salience, drop
	Kind     ThoughtKind   `json:"kind,omitempty"`     // Used by add, optional in update
	Content  string        `json:"content,omitempty"`  // Used by add, update
	Links    []string      `json:"links,omitempty"`    // Used by add, link, unlink
	Salience *float64      `json:"salience,omitempty"` // Used by add (optional), set_salience (required)
}
