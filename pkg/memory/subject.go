package memory

import (
	"strings"
	"time"
)

// SubjectKind classifies what kind of mental referent a subject is.
type SubjectKind string

const (
	SubjectPerson SubjectKind = "person"
	SubjectPlace  SubjectKind = "place"
	SubjectObject SubjectKind = "object"
	SubjectGroup  SubjectKind = "group"
	SubjectIdea   SubjectKind = "idea"
)

// BindingMode distinguishes how a mental subject corresponds to a world entity.
type BindingMode string

const (
	BindingIdentity BindingMode = "identity" // "this IS that entity"
	BindingAnchor   BindingMode = "anchor"   // "this subject is located AT/IN that entity"
)

// Binding represents an actor's fallible belief that a mental subject corresponds to a world entity.
type Binding struct {
	EntityID          string      `json:"entity_id"` // Target world entity (may be changed, destroyed, or non-existent)
	Mode              BindingMode `json:"mode"`      // identity | anchor
	Confidence        float64     `json:"confidence"`
	LastConfirmedTick int         `json:"last_confirmed_tick,omitempty"`
	LastConfirmedAt   time.Time   `json:"last_confirmed_at,omitempty"`
}

// SubjectOrigin tracks how a mental subject originated in this actor's mind.
type SubjectOrigin string

const (
	OriginPerceived SubjectOrigin = "perceived"
	OriginAuthored  SubjectOrigin = "authored"
	OriginTold      SubjectOrigin = "told" // Reserved
)

// Subject represents an entity, person, location, or idea in this actor's subjective reality.
type Subject struct {
	ID       string        `json:"id"`                 // Unique per actor (e.g. "s-zack", "s-leo", "s-home")
	Kind     SubjectKind   `json:"kind"`               // person | place | object | group | idea
	Names    []string      `json:"names"`              // Subjective aliases ("Leo", "my ex", "Mom's house")
	Bindings []Binding     `json:"bindings,omitempty"` // Believed correspondences to the world
	Origin   SubjectOrigin `json:"origin"`
}

// RecognitionResult packages the outcome of matching perception against subjective bindings.
type RecognitionResult struct {
	Recognized      map[string]float64 // subjectID -> confidence
	Anchored        map[string]float64 // subjectID -> confidence (things believed to be here)
	ExpectationMiss map[string]bool    // subjectID -> true (anchored with identity binding, but not seen!)
	EntityToSubject map[string]string  // entityID -> subjectID
}

// Recognize processes current percepts and location chain against registered subjects.
func Recognize(subjects map[string]*Subject, percepts []Percept, locationChain []string, currentTick int, currentTime time.Time) RecognitionResult {
	res := RecognitionResult{
		Recognized:      make(map[string]float64),
		Anchored:        make(map[string]float64),
		ExpectationMiss: make(map[string]bool),
		EntityToSubject: make(map[string]string),
	}

	perceivedEntities := make(map[string]Percept)
	for _, p := range percepts {
		perceivedEntities[p.EntityID] = p
	}

	locationsMap := make(map[string]bool)
	for _, loc := range locationChain {
		locationsMap[loc] = true
	}

	for _, subj := range subjects {
		for i := range subj.Bindings {
			b := &subj.Bindings[i]

			switch b.Mode {
			case BindingIdentity:
				if _, ok := perceivedEntities[b.EntityID]; ok {
					res.Recognized[subj.ID] = b.Confidence
					res.EntityToSubject[b.EntityID] = subj.ID
					b.LastConfirmedTick = currentTick
					b.LastConfirmedAt = currentTime
				}

			case BindingAnchor:
				if locationsMap[b.EntityID] {
					res.Anchored[subj.ID] = b.Confidence
				}
			}
		}

		// Detect Expectation Miss:
		// An anchored subject that also has an identity binding, but whose identity-bound entity is NOT in perceived set
		if _, isAnchored := res.Anchored[subj.ID]; isAnchored {
			hasIdentity := false
			seenIdentity := false
			for _, b := range subj.Bindings {
				if b.Mode == BindingIdentity {
					hasIdentity = true
					if _, seen := perceivedEntities[b.EntityID]; seen {
						seenIdentity = true
						break
					}
				}
			}
			if hasIdentity && !seenIdentity {
				res.ExpectationMiss[subj.ID] = true
			}
		}
	}

	return res
}

// AutoRegisterPercepts creates new perceived subjects and dossiers for entities observed at LOD >= 2 that lack bindings.
func AutoRegisterPercepts(subjects map[string]*Subject, dossiers map[string]*Dossier, percepts []Percept, currentTick int, currentTime time.Time, nextSubjectID func() string) []string {
	var newlyRegistered []string

	boundEntities := make(map[string]bool)
	for _, subj := range subjects {
		for _, b := range subj.Bindings {
			if b.Mode == BindingIdentity {
				boundEntities[b.EntityID] = true
			}
		}
	}

	for _, p := range percepts {
		if p.LOD < 2 {
			continue // Only register clear perceptions (LOD2 or LOD3)
		}
		if boundEntities[p.EntityID] {
			continue
		}

		subjID := nextSubjectID()
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = p.EntityID
		}

		subj := &Subject{
			ID:    subjID,
			Kind:  SubjectObject, // Defaults to object unless classified
			Names: []string{name},
			Bindings: []Binding{
				{
					EntityID:          p.EntityID,
					Mode:              BindingIdentity,
					Confidence:        1.0,
					LastConfirmedTick: currentTick,
					LastConfirmedAt:   currentTime,
				},
			},
			Origin: OriginPerceived,
		}
		subjects[subjID] = subj
		if dossiers != nil {
			if _, ok := dossiers[subjID]; !ok {
				dossiers[subjID] = &Dossier{SubjectID: subjID}
			}
		}
		boundEntities[p.EntityID] = true
		newlyRegistered = append(newlyRegistered, subjID)
	}

	return newlyRegistered
}
