package spec

import "encoding/json"

// Document represents the top-level root of an Omnia scenario specification.
type Document struct {
	Schema     string         `json:"$schema,omitempty" jsonschema:"description=URL or relative path to the JSON Schema contract"`
	Format     Format         `json:"format" jsonschema:"required,description=Scenario format identification"`
	Meta       Meta           `json:"meta" jsonschema:"required,description=Scenario metadata and authorship info"`
	Characters []Character    `json:"characters" jsonschema:"required,description=List of unique characters defined in this scenario"`
	Entities   []Entity       `json:"entities" jsonschema:"required,description=List of physical entities, spaces, items, and portals"`
	History    []HistoryEvent `json:"history,omitempty" jsonschema:"description=Objective historical events that occurred prior to start"`
	Minds      []Mind         `json:"minds" jsonschema:"required,description=Lifetime subjective memories and dossiers per character"`
	Starts     []Start        `json:"starts" jsonschema:"required,minItems=1,description=One or more initial starting points/snapshots"`
	Editor     json.RawMessage `json:"editor,omitempty" jsonschema:"description=Reserved object for authoring tooling or GUI metadata"`
}

// Format identifies the specification name and semantic version.
type Format struct {
	Name    string `json:"name" jsonschema:"required,enum=omnia.scenario,description=Specification identifier"`
	Version string `json:"version" jsonschema:"required,enum=1.0,description=Major.minor version string"`
}

// Meta holds document-level metadata.
type Meta struct {
	ID          string         `json:"id" jsonschema:"required,pattern=^[a-z0-9][a-z0-9_-]{0,63}$,description=Unique slug identifier for this scenario"`
	Title       string         `json:"title" jsonschema:"required,description=Human-readable scenario title"`
	Description string         `json:"description,omitempty" jsonschema:"description=Overview summary of the narrative premise"`
	Tags        []string       `json:"tags,omitempty" jsonschema:"description=Categorization tags"`
	CreatedAt   string         `json:"created_at,omitempty" jsonschema:"description=RFC 3339 creation timestamp"`
	UpdatedAt   string         `json:"updated_at,omitempty" jsonschema:"description=RFC 3339 last updated timestamp"`
	Generator   *GeneratorInfo `json:"generator,omitempty" jsonschema:"description=Tooling generator information"`
}

// GeneratorInfo describes what software authored or exported this document.
type GeneratorInfo struct {
	Name    string `json:"name" jsonschema:"required"`
	Version string `json:"version" jsonschema:"required"`
}

// Trait describes a psychological or behavioural dimension of a character.
type Trait struct {
	Name        string  `json:"name" jsonschema:"required,description=Trait name"`
	Description string  `json:"description,omitempty" jsonschema:"description=Behavioural impact of this trait"`
	Intensity   float64 `json:"intensity,omitempty" jsonschema:"minimum=0.0,maximum=1.0,description=Intensity scale 0.0 to 1.0"`
}

// Preference defines a purposeful like or dislike with its underlying motivation.
type Preference struct {
	Item   string `json:"item" jsonschema:"required,description=Thing or concept liked or disliked"`
	Reason string `json:"reason,omitempty" jsonschema:"description=Why the character feels this way (avoids generic filler)"`
}

// Character defines a person's core identity, background, and behavioural directives.
type Character struct {
	ID          string          `json:"id" jsonschema:"required,pattern=^[a-z0-9][a-z0-9_-]{0,63}$,description=Unique character slug identifier"`
	Name        string          `json:"name" jsonschema:"required,description=Full display name"`
	Aliases     []string        `json:"aliases,omitempty" jsonschema:"description=Alternative nicknames or references"`
	Personality string          `json:"personality" jsonschema:"required,description=Core personality summary"`
	Directives  []string        `json:"directives,omitempty" jsonschema:"description=Strategic goals and behavioural guardrails"`
	Background  string          `json:"background,omitempty" jsonschema:"description=Biographical history"`
	BornAt      string          `json:"born_at,omitempty" jsonschema:"description=RFC 3339 in-world date of birth"`
	Traits      []Trait         `json:"traits,omitempty" jsonschema:"description=Detailed psychological traits"`
	Likes       []Preference    `json:"likes,omitempty" jsonschema:"description=Purposeful likes"`
	Dislikes    []Preference    `json:"dislikes,omitempty" jsonschema:"description=Purposeful dislikes"`
	Editor      json.RawMessage `json:"editor,omitempty" jsonschema:"description=Tooling metadata"`
}

// Link defines how an entity attaches to its parent container or space.
type Link struct {
	Relation  string `json:"relation" jsonschema:"required,enum=in,enum=on,enum=under,enum=behind,enum=worn,enum=held,enum=seated,enum=contained,description=Spatial attachment relation"`
	Placement string `json:"placement,omitempty" jsonschema:"description=Natural language placement qualifier (e.g. 'tucked behind the pillow')"`
	Concealed bool   `json:"concealed,omitempty" jsonschema:"description=True if hidden from casual direct perception without deliberate search"`
}

// Attribute defines a dynamic physical, somatic, or psychological property on an entity.
type Attribute struct {
	Key        string   `json:"key" jsonschema:"required,description=Attribute name"`
	Value      string   `json:"value" jsonschema:"required,description=String representation of attribute value"`
	Allowed    []string `json:"allowed,omitempty" jsonschema:"description=Optional enum whitelist of valid values"`
	Core       bool     `json:"core,omitempty" jsonschema:"description=Core attributes cannot be removed at runtime"`
	Visibility string   `json:"visibility,omitempty" jsonschema:"enum=public,enum=private,description=Exposure level (public or private)"`
	Accessors  []string `json:"accessors,omitempty" jsonschema:"description=Entity IDs permitted to perceive this private attribute"`
}

// SpaceBlock configures an entity that possesses internal volume.
type SpaceBlock struct {
	Enterable   bool `json:"enterable" jsonschema:"description=True if human-scale actors can walk inside"`
	Open        bool `json:"open" jsonschema:"description=True if space allows physical entry"`
	Transparent bool `json:"transparent,omitempty" jsonschema:"description=True if boundaries allow optical visibility"`
	Soundproof  bool `json:"soundproof,omitempty" jsonschema:"description=True if acoustic events do not penetrate boundaries"`
}

// PortalBlock configures a passage or door connecting two spaces.
type PortalBlock struct {
	FromID      string `json:"from_id" jsonschema:"required,description=Origin space entity ID"`
	ToID        string `json:"to_id" jsonschema:"required,description=Destination space entity ID"`
	Direction   string `json:"direction,omitempty" jsonschema:"description=Natural language direction (e.g. 'into living room')"`
	Transparent bool   `json:"transparent,omitempty" jsonschema:"description=True if optical perception penetrates when closed"`
	Closed      bool   `json:"closed" jsonschema:"description=True if physical passage is obstructed"`
	Locked      bool   `json:"locked,omitempty" jsonschema:"description=True if portal cannot be opened without key or permission"`
}

// ActorBlock configures the bodily presence of a character in the world.
type ActorBlock struct {
	CharacterID string `json:"character_id" jsonschema:"required,description=Character record backing this body"`
	Appearance  string `json:"appearance,omitempty" jsonschema:"description=Visual description as observed by onlookers"`
	Controller  string `json:"controller,omitempty" jsonschema:"enum=llm,enum=none,description=Default controller type"`
}

// Entity represents any physical space, item, portal, or actor body in the simulation world.
type Entity struct {
	ID          string          `json:"id" jsonschema:"required,pattern=^[a-z0-9][a-z0-9_-]{0,63}$,description=Unique entity slug identifier"`
	Kind        string          `json:"kind" jsonschema:"required,enum=space,enum=item,enum=portal,enum=actor,description=Fundamental entity type"`
	Class       string          `json:"class,omitempty" jsonschema:"enum=building,enum=location,enum=room,enum=point_of_interest,enum=object,enum=container,description=Semantic classification hint"`
	Name        string          `json:"name" jsonschema:"required,description=Display name"`
	Aliases     []string        `json:"aliases,omitempty" jsonschema:"description=Alternative names for target resolution"`
	Description string          `json:"description,omitempty" jsonschema:"description=Physical sensory description"`
	Salience    float64         `json:"salience,omitempty" jsonschema:"minimum=0.05,maximum=1.0,description=Baseline perception prominence (default 0.3 for items, 0.8 for actors)"`
	ParentID    string          `json:"parent_id,omitempty" jsonschema:"description=Parent entity ID in spatial hierarchy (empty for root spaces)"`
	Link        *Link           `json:"link,omitempty" jsonschema:"description=Attachment relation to parent"`
	Attributes  []Attribute     `json:"attributes,omitempty" jsonschema:"description=Dynamic state attributes"`
	Space       *SpaceBlock     `json:"space,omitempty" jsonschema:"description=Required when kind is space"`
	Portal      *PortalBlock    `json:"portal,omitempty" jsonschema:"description=Required when kind is portal"`
	Actor       *ActorBlock     `json:"actor,omitempty" jsonschema:"description=Required when kind is actor"`
	Editor      json.RawMessage `json:"editor,omitempty" jsonschema:"description=Tooling metadata"`
}

// HistoryEvent records an objective past world event.
type HistoryEvent struct {
	ID             string          `json:"id" jsonschema:"required,pattern=^[a-z0-9][a-z0-9_-]{0,63}$,description=Unique history event slug"`
	OccurredAt     string          `json:"occurred_at" jsonschema:"required,description=RFC 3339 in-world timestamp"`
	Precision      string          `json:"precision,omitempty" jsonschema:"enum=minute,enum=hour,enum=day,enum=month,enum=year,description=Temporal granularity"`
	LocationID     string          `json:"location_id,omitempty" jsonschema:"description=Space entity ID where event occurred"`
	ParticipantIDs []string        `json:"participant_ids,omitempty" jsonschema:"description=Actor entity IDs involved"`
	Summary        string          `json:"summary" jsonschema:"required,description=Objective summary of what happened"`
	Editor         json.RawMessage `json:"editor,omitempty" jsonschema:"description=Tooling metadata"`
}

// Binding specifies a fallible mental belief that a subject corresponds to a world entity.
type Binding struct {
	EntityID   string  `json:"entity_id" jsonschema:"required,description=Target world entity ID"`
	Mode       string  `json:"mode" jsonschema:"required,enum=identity,enum=anchor,description=Belief mode (identity: this IS that entity; anchor: located AT entity)"`
	Confidence float64 `json:"confidence" jsonschema:"minimum=0.0,maximum=1.0,description=Belief confidence score"`
}

// Subject represents an entity, person, location, or idea within an actor's subjective reality.
type Subject struct {
	ID       string    `json:"id" jsonschema:"required,pattern=^[a-z0-9][a-z0-9_-]{0,63}$,description=Subject slug ID unique within this mind"`
	Kind     string    `json:"kind" jsonschema:"required,enum=person,enum=place,enum=object,enum=group,enum=idea,description=Subject category"`
	Names    []string  `json:"names" jsonschema:"required,minItems=1,description=Subjective aliases and names"`
	Origin   string    `json:"origin" jsonschema:"required,enum=authored,enum=perceived,description=Origin of this mental subject"`
	Bindings []Binding `json:"bindings,omitempty" jsonschema:"description=Believed correspondences to world entities (empty for phantoms)"`
}

// Claim represents a single subjective belief, feeling, suspicion, or intention.
type Claim struct {
	ID         string   `json:"id" jsonschema:"required,pattern=^[a-z0-9][a-z0-9_-]{0,63}$,description=Claim slug ID unique within this mind"`
	Kind       string   `json:"kind" jsonschema:"required,enum=belief,enum=feeling,enum=suspicion,enum=intention,description=Claim category"`
	Content    string   `json:"content" jsonschema:"required,description=First-person subjective statement"`
	Salience   float64  `json:"salience" jsonschema:"minimum=0.05,maximum=1.0,description=Attention prominence score"`
	Conviction float64  `json:"conviction" jsonschema:"minimum=0.0,maximum=1.0,description=Belief tenacity / certainty"`
	Fixed      bool     `json:"fixed,omitempty" jsonschema:"description=Core conviction protected from Distiller mutation and decay floor"`
	Evidence   []string `json:"evidence,omitempty" jsonschema:"description=Episode IDs in this mind supporting this claim"`
}

// Dossier bundles everything an actor subjectively believes, feels, and knows about one subject.
type Dossier struct {
	SubjectID  string  `json:"subject_id" jsonschema:"required,description=Subject ID (or owner ID for self-dossier)"`
	Impression string  `json:"impression,omitempty" jsonschema:"description=One-line summary gist (<= 25 words)"`
	Claims     []Claim `json:"claims" jsonschema:"required,description=Subjective claims held regarding this subject"`
}

// Beat represents one chronologically condensed segment within an authored episode.
type Beat struct {
	Text       string   `json:"text" jsonschema:"required,description=First-person condensed segment (<= ~60 words)"`
	SubjectIDs []string `json:"subject_ids,omitempty" jsonschema:"description=Mental subjects featured in this beat"`
}

// Episode represents an authored episodic memory owned by this character.
type Episode struct {
	ID             string   `json:"id" jsonschema:"required,pattern=^[a-z0-9][a-z0-9_-]{0,63}$,description=Episode slug ID unique within this mind"`
	Start          string   `json:"start" jsonschema:"required,description=RFC 3339 in-world start timestamp"`
	End            string   `json:"end" jsonschema:"required,description=RFC 3339 in-world end timestamp"`
	Precision      string   `json:"precision,omitempty" jsonschema:"enum=minute,enum=hour,enum=day,enum=month,enum=year,description=Temporal granularity"`
	Label          string   `json:"label,omitempty" jsonschema:"description=Fuzzy temporal label (e.g. 'the summer I turned twelve')"`
	PlaceIDs       []string `json:"place_ids,omitempty" jsonschema:"description=Subject IDs of places actor believed they were"`
	ParticipantIDs []string `json:"participant_ids,omitempty" jsonschema:"description=Subject IDs of characters perceived"`
	Title          string   `json:"title" jsonschema:"required,description=Short episode title (<= 12 words)"`
	Summary        string   `json:"summary" jsonschema:"required,description=First-person narrative overview (<= ~80 words)"`
	Tone           string   `json:"tone,omitempty" jsonschema:"description=Emotional atmosphere (e.g. 'warm and hopeful')"`
	Importance     float64  `json:"importance" jsonschema:"minimum=0.05,maximum=1.0,description=Formative importance score"`
	Beats          []Beat   `json:"beats" jsonschema:"required,minItems=1,description=Chronological chain of condensed beats"`
}

// Mind captures the lifetime subjective reality, archive episodes, and dossiers of a character.
type Mind struct {
	OwnerID  string    `json:"owner_id" jsonschema:"required,description=Character ID who owns this mind"`
	Subjects []Subject `json:"subjects" jsonschema:"required,description=Mental subjects known to this character"`
	Dossiers []Dossier `json:"dossiers" jsonschema:"required,description=Subjective dossiers and claims held by this character"`
	Episodes []Episode `json:"episodes" jsonschema:"required,description=Authored episodic memories"`
}

// Clock defines the simulation clock configuration at a start point.
type Clock struct {
	Cycle        int    `json:"cycle" jsonschema:"required,minimum=1,description=Simulation day or cycle number"`
	StartAt      string `json:"start_at" jsonschema:"required,description=RFC 3339 simulation starting time"`
	TickSeconds  int    `json:"tick_seconds" jsonschema:"required,minimum=1,description=Simulated elapsed seconds per tick"`
}

// EntityOverride specifies start-time modifications to an entity.
type EntityOverride struct {
	EntityID         string       `json:"entity_id" jsonschema:"required,description=Target entity ID"`
	ParentID         string       `json:"parent_id,omitempty" jsonschema:"description=Replacement parent space or container"`
	Link             *Link        `json:"link,omitempty" jsonschema:"description=Replacement spatial link"`
	Salience         *float64     `json:"salience,omitempty" jsonschema:"minimum=0.05,maximum=1.0,description=Override salience"`
	Space            *SpaceBlock  `json:"space,omitempty" jsonschema:"description=Override space block parameters"`
	Portal           *PortalBlock `json:"portal,omitempty" jsonschema:"description=Override portal parameters (e.g. closed/locked)"`
	SetAttributes    []Attribute  `json:"set_attributes,omitempty" jsonschema:"description=Attributes to set or replace"`
	RemoveAttributes []string     `json:"remove_attributes,omitempty" jsonschema:"description=Non-core attribute keys to remove"`
}

// ClaimUpsert specifies a claim to insert or replace in a start-specific mind overlay.
type ClaimUpsert struct {
	SubjectID string `json:"subject_id" jsonschema:"required,description=Subject ID in owner's mind"`
	Claim     Claim  `json:"claim" jsonschema:"required,description=Claim to insert or update"`
}

// MindOverride specifies start-specific adjustments to a character's mind.
type MindOverride struct {
	OwnerID           string        `json:"owner_id" jsonschema:"required,description=Character ID whose mind is being modified"`
	EpisodeExcludeIDs []string      `json:"episode_exclude_ids,omitempty" jsonschema:"description=Lifetime episode IDs to exclude from this start"`
	UpsertClaims      []ClaimUpsert `json:"upsert_claims,omitempty" jsonschema:"description=Start-specific intentions, feelings, or beliefs"`
	RetractClaimIDs   []string      `json:"retract_claim_ids,omitempty" jsonschema:"description=Claim IDs to retract at this start"`
	UpsertSubjects    []Subject     `json:"upsert_subjects,omitempty" jsonschema:"description=Subjects to insert or replace for this start"`
}

// EventSchedule defines an external narrative event scheduled for a specific simulation tick.
type EventSchedule struct {
	ID         string `json:"id" jsonschema:"required,pattern=^[a-z0-9][a-z0-9_-]{0,63}$,description=Unique event slug identifier"`
	Tick       int    `json:"tick" jsonschema:"required,minimum=1,description=Simulation tick number when this event triggers"`
	LocationID string `json:"location_id" jsonschema:"required,description=Space entity ID where the event is broadcast"`
	Text       string `json:"text" jsonschema:"required,description=Sensory description of what occurs in the space"`
}

// Start defines an authored simulation starting point.
type Start struct {
	ID         string           `json:"id" jsonschema:"required,pattern=^[a-z0-9][a-z0-9_-]{0,63}$,description=Unique start point slug identifier"`
	Name       string           `json:"name" jsonschema:"required,description=Display title for this starting point"`
	Clock      Clock            `json:"clock" jsonschema:"required,description=Initial clock parameters"`
	Seed       *int64           `json:"seed,omitempty" jsonschema:"description=Optional fixed PRNG seed for deterministic runs"`
	PlayerID   string           `json:"player_id,omitempty" jsonschema:"description=Optional character ID assigned to the human player"`
	PresentIDs []string         `json:"present_ids,omitempty" jsonschema:"description=Entity IDs present at this start (empty means all entities)"`
	Overrides  []EntityOverride `json:"overrides,omitempty" jsonschema:"description=Per-entity initial state overrides"`
	Minds      []MindOverride   `json:"minds,omitempty" jsonschema:"description=Start-specific mental state overlays"`
	Events     []EventSchedule  `json:"events,omitempty" jsonschema:"description=Scheduled initial sensory events"`
}
