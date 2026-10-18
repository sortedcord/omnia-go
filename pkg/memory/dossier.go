package memory

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// ClaimKind classifies a subjective statement within a dossier.
type ClaimKind string

const (
	ClaimBelief    ClaimKind = "belief"
	ClaimFeeling   ClaimKind = "feeling"
	ClaimSuspicion ClaimKind = "suspicion"
	ClaimIntention ClaimKind = "intention"
)

// LinkRole specifies how an episode links to a subject dossier.
type LinkRole string

const (
	RoleParticipant LinkRole = "participant"
	RolePlace       LinkRole = "place"
	RoleEvidence    LinkRole = "evidence"
	RoleFormative   LinkRole = "formative"
	RoleMentioned   LinkRole = "mentioned"
)

// Revision preserves prior contents when a claim is updated or retracted.
type Revision struct {
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Tick      int       `json:"tick"`
}

// Claim represents a single subjective belief, feeling, suspicion, or intention.
type Claim struct {
	ID         string     `json:"id"`                 // e.g. "c-nelly-zack-1"
	Kind       ClaimKind  `json:"kind"`               // belief | feeling | suspicion | intention
	Content    string     `json:"content"`            // First-person subjective text
	Salience   float64    `json:"salience"`           // Attention priority: 0.05 to 1.00
	Conviction float64    `json:"conviction"`         // Tenacity/certainty: 0.00 to 1.00
	Fixed      bool       `json:"fixed,omitempty"`    // Delusion/core conviction; protected from Distiller mutation
	Evidence   []string   `json:"evidence,omitempty"` // Episode IDs supporting this claim
	Formed     int        `json:"formed_tick"`
	Updated    int        `json:"updated_tick"`
	History    []Revision `json:"history,omitempty"` // Up to 5 prior versions
}

// EpisodeLink connects a dossier to an episodic memory that formed or confirmed its claims.
type EpisodeLink struct {
	EpisodeID string   `json:"episode_id"`
	Weight    float64  `json:"weight"` // 0.0 to 1.0
	Role      LinkRole `json:"role"`   // participant | place | evidence | formative | mentioned
}

// Dossier represents everything an actor subjectively believes, feels, and knows about one subject.
type Dossier struct {
	SubjectID  string        `json:"subject_id"` // Matches Subject.ID, or owner ID for self-dossier
	Impression string        `json:"impression"` // One-line summary gist (<= 25 words)
	Claims     []Claim       `json:"claims"`
	Links      []EpisodeLink `json:"links,omitempty"`
	Updated    int           `json:"updated_tick"`
}

// DecayClaims applies kind-specific temporal decay to claims based on elapsed in-world time.
func (d *Dossier) DecayClaims(currentTime time.Time, lastDecayTime time.Time) {
	if lastDecayTime.IsZero() || currentTime.Before(lastDecayTime) {
		return
	}
	elapsed := currentTime.Sub(lastDecayTime)
	if elapsed <= 0 {
		return
	}

	var active []Claim
	for _, c := range d.Claims {
		if c.Fixed {
			if c.Salience < 0.50 {
				c.Salience = 0.50
			}
			active = append(active, c)
			continue
		}

		halfLife := getClaimHalfLife(c.Kind)
		floor := getClaimFloor(c.Kind)

		periods := float64(elapsed) / float64(halfLife)
		factor := math.Pow(0.5, periods)
		newSalience := c.Salience * factor

		if newSalience < floor {
			newSalience = floor
		}

		// Claims fading below 0.08 without a floor are archived (excluded from active prompt)
		if newSalience >= 0.08 || floor > 0 {
			c.Salience = newSalience
			active = append(active, c)
		}
	}
	d.Claims = active
}

func getClaimHalfLife(kind ClaimKind) time.Duration {
	switch kind {
	case ClaimFeeling:
		return 15 * time.Minute
	case ClaimIntention:
		return 2 * time.Hour
	case ClaimSuspicion:
		return 72 * time.Hour // 3 days
	case ClaimBelief:
		return 30 * 24 * time.Hour // 30 days
	default:
		return 24 * time.Hour
	}
}

func getClaimFloor(kind ClaimKind) float64 {
	switch kind {
	case ClaimSuspicion:
		return 0.10
	case ClaimBelief:
		return 0.15
	default:
		return 0.0
	}
}

// AddLink inserts or reinforces an episode link in the dossier, capping total links at 40.
func (d *Dossier) AddLink(episodeID string, weight float64, role LinkRole) {
	for i := range d.Links {
		if d.Links[i].EpisodeID == episodeID {
			if weight > d.Links[i].Weight {
				d.Links[i].Weight = weight
				d.Links[i].Role = role
			}
			return
		}
	}

	d.Links = append(d.Links, EpisodeLink{
		EpisodeID: episodeID,
		Weight:    weight,
		Role:      role,
	})

	// Cap at top 40 links by weight
	if len(d.Links) > 40 {
		sort.Slice(d.Links, func(i, j int) bool {
			return d.Links[i].Weight > d.Links[j].Weight
		})
		d.Links = d.Links[:40]
	}
}

// DossierOp defines an atomic mutation applied by the Thought Distiller.
type DossierOp struct {
	Op          string      `json:"op"` // upsert_claim | retract_claim | set_impression | add_name | new_subject | bind | unbind
	SubjectID   string      `json:"subject_id"`
	ClaimID     string      `json:"claim_id,omitempty"`
	Kind        ClaimKind   `json:"kind,omitempty"`
	Content     string      `json:"content,omitempty"`
	Salience    *float64    `json:"salience,omitempty"`
	Conviction  *float64    `json:"conviction,omitempty"`
	Name        string      `json:"name,omitempty"`         // For add_name or new_subject
	SubjectKind SubjectKind `json:"subject_kind,omitempty"` // For new_subject
	EntityID    string      `json:"entity_id,omitempty"`    // For bind or unbind
	BindMode    BindingMode `json:"bind_mode,omitempty"`    // For bind (identity | anchor)
}

// ApplyOp applies a single DossierOp to the dossier and subject registry with strict Fixed protection.
func (d *Dossier) ApplyOp(tick int, op DossierOp, openEpisodeID string) error {
	switch op.Op {
	case "set_impression":
		d.Impression = strings.TrimSpace(op.Content)
		d.Updated = tick
		return nil

	case "retract_claim":
		for i, c := range d.Claims {
			if c.ID == op.ClaimID {
				if c.Fixed {
					return fmt.Errorf("cannot retract fixed claim %q", c.ID)
				}
				// Archive into history before removing
				c.History = append(c.History, Revision{
					Content: c.Content,
					Tick:    tick,
				})
				d.Claims = append(d.Claims[:i], d.Claims[i+1:]...)
				d.Updated = tick
				return nil
			}
		}
		return fmt.Errorf("claim %q not found for retraction", op.ClaimID)

	case "upsert_claim":
		salience := 0.75
		if op.Salience != nil {
			salience = *op.Salience
			if salience < 0.05 {
				salience = 0.05
			} else if salience > 1.00 {
				salience = 1.00
			}
		}

		conviction := 0.80
		if op.Conviction != nil {
			conviction = *op.Conviction
			if conviction < 0.00 {
				conviction = 0.00
			} else if conviction > 1.00 {
				conviction = 1.00
			}
		}

		// Update existing claim if ID provided
		if op.ClaimID != "" {
			for i := range d.Claims {
				if d.Claims[i].ID == op.ClaimID {
					if d.Claims[i].Fixed {
						return fmt.Errorf("cannot update fixed claim %q", op.ClaimID)
					}
					// Record revision in history
					d.Claims[i].History = append(d.Claims[i].History, Revision{
						Content: d.Claims[i].Content,
						Tick:    tick,
					})
					if len(d.Claims[i].History) > 5 {
						d.Claims[i].History = d.Claims[i].History[len(d.Claims[i].History)-5:]
					}

					if op.Content != "" {
						d.Claims[i].Content = strings.TrimSpace(op.Content)
					}
					if op.Kind != "" {
						d.Claims[i].Kind = op.Kind
					}
					d.Claims[i].Salience = salience
					d.Claims[i].Conviction = conviction
					d.Claims[i].Updated = tick

					if openEpisodeID != "" {
						d.Claims[i].Evidence = addUniqueString(d.Claims[i].Evidence, openEpisodeID)
						d.AddLink(openEpisodeID, 0.8, RoleEvidence)
					}
					d.Updated = tick
					return nil
				}
			}
		}

		// Insert new claim
		newClaim := Claim{
			ID:         op.ClaimID,
			Kind:       op.Kind,
			Content:    strings.TrimSpace(op.Content),
			Salience:   salience,
			Conviction: conviction,
			Formed:     tick,
			Updated:    tick,
		}
		if newClaim.Kind == "" {
			newClaim.Kind = ClaimBelief
		}
		if openEpisodeID != "" {
			newClaim.Evidence = []string{openEpisodeID}
			d.AddLink(openEpisodeID, 0.8, RoleEvidence)
		}
		d.Claims = append(d.Claims, newClaim)
		d.Updated = tick
		return nil

	default:
		return nil
	}
}

func addUniqueString(list []string, item string) []string {
	for _, s := range list {
		if s == item {
			return list
		}
	}
	return append(list, item)
}
