package sim

import "omnia.adityagupta.dev/pkg/actor"

// EntityID is the unique identifier for any entity in the simulation world.
type EntityID string

// Relation describes how a child entity is spatially attached to its parent.
type Relation string

const (
	RelIn        Relation = "in"
	RelOn        Relation = "on"
	RelUnder     Relation = "under"
	RelBehind    Relation = "behind"
	RelWorn      Relation = "worn"
	RelHeld      Relation = "held"
	RelSeated    Relation = "seated"
	RelContained Relation = "contained"
)

// Link specifies the spatial relationship and visibility between a child and its parent entity.
type Link struct {
	Relation  Relation `json:"relation"`
	Placement string   `json:"placement,omitempty"` // Free-text qualifier, e.g. "tucked behind the founder's portrait"
	Concealed bool     `json:"concealed,omitempty"` // True if hidden from direct perception without deliberate search
}

// Space gives an entity an interior that can hold other entities.
// If Enterable is true, actors can enter and reside inside this space (e.g. rooms, vehicle cabins).
// If Enterable is false, it acts as a pocket, drawer, or container box.
type Space struct {
	Enterable   bool              `json:"enterable"`
	Open        bool              `json:"open"`
	Transparent bool              `json:"transparent"`
	Soundproof  bool              `json:"soundproof"`
	Conditions  map[string]string `json:"conditions,omitempty"` // e.g. "Lighting": "Pitch Black", "Atmosphere": "Nominal"
}

// Portal connects two spaces through an opening or barrier (e.g. a door, hatch, window, or corridor).
type Portal struct {
	Target      EntityID `json:"target"`
	Direction   string   `json:"direction,omitempty"` // e.g. "north", "up", "bulkhead hatch"
	Transparent bool     `json:"transparent"`
	Closed      bool     `json:"closed"`
	Locked      bool     `json:"locked"`
}

// Entity is the universal building block for everything in the simulation:
// a room, a vehicle, an actor, a piece of clothing, a pocket, or a keycard.
// Behavior and capabilities are governed by optional component pointers.
type Entity struct {
	ID          EntityID `json:"id"`
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	Description string   `json:"description,omitempty"`
	Salience    float64  `json:"salience"` // Intrinsic importance baseline (0.0 to 1.0)

	// Spatial Containment Tree
	Parent   EntityID   `json:"parent,omitempty"`
	Link     Link       `json:"link"`
	Children []EntityID `json:"children,omitempty"`

	// Optional Capabilities (nil = capability absent)
	Space      *Space       `json:"space,omitempty"`
	Portal     *Portal      `json:"portal,omitempty"`
	Mind       *actor.Actor `json:"-"` // Active subjective character state (profile, memory, somatic status)
	Controller Controller   `json:"-"` // Directs decisions (nil = passive entity, non-nil = LLM or Human)

	// Subtree Summary Cache (for LOD 1 rendering)
	Summary    string `json:"summary,omitempty"`
	SummaryRev int    `json:"summary_rev"`
	Revision   int    `json:"revision"`
}

// NewEntity creates a base entity with safe defaults.
func NewEntity(id EntityID, name string) *Entity {
	return &Entity{
		ID:       id,
		Name:     name,
		Salience: 0.2, // standard prop baseline
		Children: make([]EntityID, 0),
		Revision: 1,
	}
}

// NewSpaceEntity creates an entity that possesses an internal space (e.g. a room, cabin, or container).
// By default, spaces are opaque unless explicitly marked Transparent.
func NewSpaceEntity(id EntityID, name, description string, enterable bool) *Entity {
	e := NewEntity(id, name)
	e.Description = description
	e.Salience = 0.5
	e.Space = &Space{
		Enterable:   enterable,
		Open:        true,
		Transparent: false, // Opaque by default; set to true for glass viewports, vehicle windows, etc.
		Soundproof:  false,
		Conditions:  make(map[string]string),
	}
	return e
}

// NewActorEntity creates an entity housing an autonomous mind.
// visualDescription is what onlookers physically see (not internal psychological personality).
func NewActorEntity(id EntityID, a *actor.Actor, visualDescription string) *Entity {
	e := NewEntity(id, a.Profile.Name)
	e.Aliases = []string{a.Profile.Name}
	if visualDescription != "" {
		e.Description = visualDescription
	} else {
		e.Description = a.Profile.Name
	}
	e.Salience = 0.8 // actors have high baseline salience
	e.Mind = a
	e.Controller = NewLLMController()
	return e
}

// NewItemEntity creates a physical item entity.
func NewItemEntity(id EntityID, name, description string) *Entity {
	e := NewEntity(id, name)
	e.Description = description
	e.Aliases = []string{name}
	e.Salience = 0.3
	return e
}

// NewPortalEntity creates a portal connecting to a target space.
func NewPortalEntity(id EntityID, name string, target EntityID, direction string) *Entity {
	e := NewEntity(id, name)
	e.Salience = 0.4
	e.Portal = &Portal{
		Target:    target,
		Direction: direction,
		Closed:    false,
		Locked:    false,
	}
	return e
}
