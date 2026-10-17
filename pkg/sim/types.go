package sim

import (
	"fmt"
	"time"

	"omnia.adityagupta.dev/pkg/actor"
)

// Clock tracks the simulation's temporal progression.
type Clock struct {
	Cycle        int
	StartTime    time.Time
	TickDuration time.Duration
	TickCount    int
}

// CurrentTime computes the exact simulated time for the current tick.
func (c Clock) CurrentTime() time.Time {
	if c.StartTime.IsZero() {
		return time.Time{}
	}
	offset := time.Duration(c.TickCount-1) * c.TickDuration
	return c.StartTime.Add(offset)
}

// Timestamp formats the current simulation time as "15:04:05 UTC" (or "15:04 UTC" if tick duration is whole minutes).
func (c Clock) Timestamp() string {
	t := c.CurrentTime()
	if t.IsZero() {
		return fmt.Sprintf("Tick %d", c.TickCount)
	}
	if c.TickDuration >= time.Minute && c.TickDuration%time.Minute == 0 {
		return t.Format("15:04 UTC")
	}
	return t.Format("15:04:05 UTC")
}

// String formats the current simulation temporal state.
func (c Clock) String() string {
	ts := c.Timestamp()
	if c.Cycle > 0 {
		return fmt.Sprintf("Cycle %d, %s (Tick %d)", c.Cycle, ts, c.TickCount)
	}
	return fmt.Sprintf("%s (Tick %d)", ts, c.TickCount)
}

// Advance advances the clock by a number of ticks.
func (c *Clock) Advance(ticks int) {
	c.TickCount += ticks
}

// EventKind classifies the origin/nature of an event in the simulation.
type EventKind string

const (
	EventWorld   EventKind = "world"   // Environmental or scripted world occurrence
	EventSpeak   EventKind = "speak"   // Spoken aloud by an actor
	EventAct     EventKind = "act"     // Physical action attempted by an actor and resolved by Arbiter
	EventWait    EventKind = "wait"    // Pausing/observing
	EventOutcome EventKind = "outcome" // Personal sensory feedback delivered to an actor from their own action
)

// Event is an objective log entry of something that happened in the simulation world.
type Event struct {
	Tick        int       `json:"tick"`
	Timestamp   string    `json:"timestamp"`
	LocationID  EntityID  `json:"location_id"`
	SceneID     SceneID   `json:"scene_id,omitempty"`
	SourceID    EntityID  `json:"source_id,omitempty"`   // Empty for environmental/world events
	SourceName  string    `json:"source_name,omitempty"` // Display name of source entity
	Kind        EventKind `json:"kind"`
	Description string    `json:"description"`        // Objective canonical description
	Observed    string    `json:"observed,omitempty"` // What co-present onlookers witness
	Outcome     string    `json:"outcome,omitempty"`  // Ruling outcome: success, partial, failure, mishap, interrupted
	Intent      string    `json:"intent,omitempty"`   // Original declared intent of the actor
}

// FormatForPerception formats this event as perceived by a co-present observer in the room.
func (e Event) FormatForPerception() string {
	switch e.Kind {
	case EventSpeak:
		return fmt.Sprintf("%s speaks aloud: %q", e.SourceName, e.Description)
	case EventAct:
		if e.Observed != "" {
			return fmt.Sprintf("%s: %s", e.SourceName, e.Observed)
		}
		return fmt.Sprintf("%s performs an action: %s", e.SourceName, e.Description)
	case EventOutcome, EventWorld:
		return e.Description
	default:
		return fmt.Sprintf("[%s] %s", e.SourceName, e.Description)
	}
}

// TickReport summarizes an actor's reaction during a simulation tick.
type TickReport struct {
	Tick         int
	Timestamp    string
	SceneID      SceneID
	ActorID      EntityID
	ActorName    string
	Stimulus     string
	Reaction     *actor.Reaction
	SceneRuling  *SceneRuling    // The Arbiter's ruling for this actor's scene
	Thoughts     []actor.Thought // Active thoughts selected into context for this turn
	PriorDistill *DistillResult  // Results of background distillation from previous turn
}
