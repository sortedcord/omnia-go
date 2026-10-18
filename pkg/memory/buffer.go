package memory

import (
	"encoding/json"
	"strings"
	"time"
)

// Percept captures an entity observed in sensory perception during a turn.
type Percept struct {
	EntityID string `json:"entity_id"`
	LOD      int    `json:"lod"`
	Name     string `json:"name"`
}

// ActionRecord represents an actor's manifest action emitted into the world.
type ActionRecord struct {
	Type    string   `json:"type"` // "speak", "act", "wait"
	Action  string   `json:"action"`
	Targets []string `json:"targets,omitempty"`
}

// Turn captures one interaction cycle from the actor's subjective viewpoint.
type Turn struct {
	Seq        int64        `json:"seq"`      // Monotonic sequence number per actor
	Tick       int          `json:"tick"`     // Simulation tick
	Time       time.Time    `json:"time"`     // Simulated in-world timestamp
	SceneID    string       `json:"scene_id"` // Episode boundary key
	LocationID string       `json:"location_id"`
	Perception string       `json:"perception"` // Subjective sensory text
	Perceived  []Percept    `json:"perceived,omitempty"`
	Cognition  string       `json:"cognition,omitempty"` // Cleaned private reasoning (not replayed in prompt)
	Action     ActionRecord `json:"action"`
	EpisodeID  string       `json:"episode_id,omitempty"` // Open episode this turn belongs to
}

// WorkingBuffer manages Tier 1 verbatim recent interaction history.
type WorkingBuffer struct {
	turns               []Turn
	budgetTokens        int   // Max token budget before consolidation is triggered (default 2500)
	minVerbatimTurns    int   // Minimum recent turns guaranteed verbatim (default 4)
	lastConsolidatedSeq int64 // Watermark for committed consolidation
	nextSeq             int64 // Next monotonic sequence number
}

// NewWorkingBuffer initializes an empty working buffer with configured token budget.
func NewWorkingBuffer(budgetTokens, minVerbatimTurns int) *WorkingBuffer {
	if budgetTokens <= 0 {
		budgetTokens = 2500
	}
	if minVerbatimTurns <= 0 {
		minVerbatimTurns = 4
	}
	return &WorkingBuffer{
		turns:            make([]Turn, 0),
		budgetTokens:     budgetTokens,
		minVerbatimTurns: minVerbatimTurns,
		nextSeq:          1,
	}
}

// Append adds a new turn to the buffer with an assigned sequence number.
func (b *WorkingBuffer) Append(t Turn) int64 {
	t.Seq = b.nextSeq
	b.nextSeq++
	b.turns = append(b.turns, t)
	return t.Seq
}

// Len returns the number of turns currently held in the buffer.
func (b *WorkingBuffer) Len() int {
	return len(b.turns)
}

// LastSeq returns the highest sequence number in the buffer, or 0 if empty.
func (b *WorkingBuffer) LastSeq() int64 {
	if len(b.turns) == 0 {
		return 0
	}
	return b.turns[len(b.turns)-1].Seq
}

// LastConsolidatedSeq returns the watermark of the last consolidated turn.
func (b *WorkingBuffer) LastConsolidatedSeq() int64 {
	return b.lastConsolidatedSeq
}

// AllTurns returns a shallow copy of all turns currently in the buffer.
func (b *WorkingBuffer) AllTurns() []Turn {
	res := make([]Turn, len(b.turns))
	copy(res, b.turns)
	return res
}

// ReplayTurns returns the recent turns to include in the LLM chat history.
// It respects the token budget while guaranteeing at least minVerbatimTurns.
func (b *WorkingBuffer) ReplayTurns() []Turn {
	if len(b.turns) <= b.minVerbatimTurns {
		res := make([]Turn, len(b.turns))
		copy(res, b.turns)
		return res
	}

	// Always include at least minVerbatimTurns
	startIdx := len(b.turns) - b.minVerbatimTurns

	// Walk backwards to include as many turns as fit within budgetTokens
	currentTokens := 0
	for i := len(b.turns) - 1; i >= 0; i-- {
		tTokens := estimateTurnTokens(b.turns[i])
		if i < startIdx && currentTokens+tTokens > b.budgetTokens {
			break
		}
		currentTokens += tTokens
		startIdx = i
	}

	res := make([]Turn, len(b.turns)-startIdx)
	copy(res, b.turns[startIdx:])
	return res
}

// EstimatedTokens returns the total estimated token volume of all turns in the buffer.
func (b *WorkingBuffer) EstimatedTokens() int {
	total := 0
	for _, t := range b.turns {
		total += estimateTurnTokens(t)
	}
	return total
}

// NeedsConsolidation returns true if the buffer exceeds budget or has uncommitted turns.
func (b *WorkingBuffer) NeedsConsolidation() bool {
	return b.EstimatedTokens() > b.budgetTokens && b.UnconsolidatedCount() > b.minVerbatimTurns
}

// UnconsolidatedTurns returns turns with Seq > lastConsolidatedSeq.
func (b *WorkingBuffer) UnconsolidatedTurns() []Turn {
	var res []Turn
	for _, t := range b.turns {
		if t.Seq > b.lastConsolidatedSeq {
			res = append(res, t)
		}
	}
	return res
}

// UnconsolidatedCount returns the number of turns waiting for consolidation.
func (b *WorkingBuffer) UnconsolidatedCount() int {
	count := 0
	for _, t := range b.turns {
		if t.Seq > b.lastConsolidatedSeq {
			count++
		}
	}
	return count
}

// CommitConsolidation advances the watermark and evicts consolidated turns that exceed budget.
func (b *WorkingBuffer) CommitConsolidation(toSeq int64) {
	if toSeq > b.lastConsolidatedSeq {
		b.lastConsolidatedSeq = toSeq
	}

	// Only evict turns that are both:
	// 1. Consolidated (Seq <= lastConsolidatedSeq)
	// 2. Beyond minVerbatimTurns from the end
	if len(b.turns) <= b.minVerbatimTurns {
		return
	}

	keepFrom := len(b.turns) - b.minVerbatimTurns
	var active []Turn
	for i, t := range b.turns {
		if i >= keepFrom || t.Seq > b.lastConsolidatedSeq {
			active = append(active, t)
		}
	}
	b.turns = active
}

// DegradedEvictionFallback handles emergency eviction when the consolidator repeatedly fails
// and the buffer exceeds 2x budget. It condenses evicted turns mechanically into a degraded beat.
func (b *WorkingBuffer) DegradedEvictionFallback() (evicted []Turn, beatText string, fromSeq, toSeq int64) {
	if len(b.turns) <= b.minVerbatimTurns {
		return nil, "", 0, 0
	}

	evictCount := len(b.turns) - b.minVerbatimTurns
	evicted = make([]Turn, evictCount)
	copy(evicted, b.turns[:evictCount])
	b.turns = append([]Turn(nil), b.turns[evictCount:]...)

	fromSeq = evicted[0].Seq
	toSeq = evicted[len(evicted)-1].Seq
	b.lastConsolidatedSeq = toSeq

	var sb strings.Builder
	sb.WriteString("[Degraded Consolidation Summary]: ")
	for i, t := range evicted {
		if i > 0 {
			sb.WriteString(" Then, ")
		}
		// First sentence of perception
		firstSentence := strings.Split(t.Perception, ".")[0]
		firstSentence = strings.TrimSpace(firstSentence)
		if firstSentence != "" {
			sb.WriteString(firstSentence)
			sb.WriteString(". ")
		}
		if t.Action.Action != "" {
			sb.WriteString(t.Action.Action)
		}
	}
	beatText = sb.String()
	return evicted, beatText, fromSeq, toSeq
}

func estimateTurnTokens(t Turn) int {
	chars := len(t.Perception) + len(t.Cognition) + len(t.Action.Action)
	for _, tgt := range t.Action.Targets {
		chars += len(tgt)
	}
	tokens := chars / 4
	if tokens < 10 {
		return 10
	}
	return tokens
}

// ActionToJSON converts an ActionRecord into formatted JSON for replay.
func (a ActionRecord) ActionToJSON() string {
	b, _ := json.Marshal(map[string]any{
		"action_type": a.Type,
		"action":      a.Action,
		"targets":     a.Targets,
	})
	return string(b)
}
