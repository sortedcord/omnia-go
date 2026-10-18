package memory

import (
	"fmt"
	"time"
)

// Source represents the origin of an episodic memory.
type Source string

const (
	SourceLived    Source = "lived"
	SourceAuthored Source = "authored"
	SourceHearsay  Source = "hearsay" // Reserved
)

// EpisodeStatus represents the lifecycle state of an episode.
type EpisodeStatus string

const (
	EpisodeOpen   EpisodeStatus = "open"
	EpisodeClosed EpisodeStatus = "closed"
)

// Precision specifies the temporal granularity of an episode's timestamp.
type Precision string

const (
	PrecisionMinute Precision = "minute"
	PrecisionHour   Precision = "hour"
	PrecisionDay    Precision = "day"
	PrecisionMonth  Precision = "month"
	PrecisionYear   Precision = "year"
)

// Beat represents one chronologically condensed segment within an episode.
type Beat struct {
	FromSeq    int64    `json:"from_seq"`
	ToSeq      int64    `json:"to_seq"`
	Text       string   `json:"text"`                  // First-person condensed segment (<= ~60 words)
	SubjectIDs []string `json:"subject_ids,omitempty"` // Mental subjects featured in this beat
	Degraded   bool     `json:"degraded,omitempty"`    // True if produced by emergency mechanical fallback
}

// Episode represents one continuous scene or authored memory as experienced by this actor.
type Episode struct {
	ID             string        `json:"id"`                        // e.g. "ep-nelly-1"
	OwnerID        string        `json:"owner_id"`                  // Actor who owns this subjective recollection
	Source         Source        `json:"source"`                    // lived | authored | hearsay
	Status         EpisodeStatus `json:"status"`                    // open | closed
	SceneID        string        `json:"scene_id,omitempty"`        // Active scene ID while open
	PrevID         string        `json:"prev_id,omitempty"`         // Previous episode in chronological chain
	Start          time.Time     `json:"start"`                     // In-world start time
	End            time.Time     `json:"end"`                       // In-world end time
	Precision      Precision     `json:"precision"`                 // Temporal resolution
	Label          string        `json:"label,omitempty"`           // e.g. "the summer I turned twelve"
	PlaceIDs       []string      `json:"place_ids,omitempty"`       // Subject IDs where actor believed they were
	ParticipantIDs []string      `json:"participant_ids,omitempty"` // Subject IDs of perceived actors
	PerceivedIDs   []string      `json:"perceived_ids,omitempty"`   // Raw world entity IDs perceived (provenance only)
	Title          string        `json:"title"`                     // Short title (<= 12 words)
	Summary        string        `json:"summary"`                   // First-person overview (<= ~80 words)
	Beats          []Beat        `json:"beats"`                     // Chronological chain of condensed beats
	Tone           string        `json:"tone,omitempty"`            // Emotional atmosphere (e.g. "mortified relief")
	Importance     float64       `json:"importance"`                // Formative scale: 0.05 to 1.00

	// Recall bookkeeping (mutable after close)
	RecallCount      int       `json:"recall_count"`
	LastRecalledTick int       `json:"last_recalled_tick,omitempty"`
	LastRecalledAt   time.Time `json:"last_recalled_at,omitempty"`
}

// FormatSummary formats an episode into a concise, evocative memory representation for prompt injection.
func (e *Episode) FormatSummary() string {
	dateStr := e.Start.Format("2006-01-02 15:04")
	if e.Label != "" {
		dateStr = fmt.Sprintf("%s (%s)", e.Label, dateStr)
	}

	places := ""
	if len(e.PlaceIDs) > 0 {
		places = fmt.Sprintf(" at [%s]", e.PlaceIDs[0])
	}

	toneStr := ""
	if e.Tone != "" {
		toneStr = fmt.Sprintf(" | Feeling: %s", e.Tone)
	}

	return fmt.Sprintf("- [%s: %s%s%s]\n  %s", e.ID, e.Title, places, toneStr, e.Summary)
}

// FormatVivid formats an episode in full detail including its chronological beats.
func (e *Episode) FormatVivid() string {
	base := e.FormatSummary()
	if len(e.Beats) == 0 {
		return base
	}

	var beatLines string
	for i, b := range e.Beats {
		beatLines += fmt.Sprintf("\n    (%d) %s", i+1, b.Text)
	}
	return base + "\n  Beats:" + beatLines
}
