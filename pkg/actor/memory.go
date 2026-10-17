package actor

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// TurnMemory records a single interaction cycle from the actor's subjective viewpoint.
type TurnMemory struct {
	Timestamp string
	Stimulus  string // What the actor perceived
	Cognition string // What the actor thought internally (private reasoning)
	Action    Action // What the actor did or said outwardly
}

// Memory stores the actor's subjective history, mutable thought graph, and known entities.
type Memory struct {
	mu sync.RWMutex

	// Chronological history of perceived turns and actions
	Turns []TurnMemory

	// MaxHistoryTurns bounds how many historical turns are fed into the chat context
	MaxHistoryTurns int

	// Mutable thoughts graph
	Thoughts      []Thought
	nextThoughtID int
	decayedAt     int

	// Entities the actor has subjectively perceived or has explicit beliefs about (ID -> Name)
	KnownEntities map[string]string
}

// NewMemory initializes an empty subjective memory bank.
func NewMemory() *Memory {
	return &Memory{
		Turns:           make([]TurnMemory, 0),
		MaxHistoryTurns: 10,
		Thoughts:        make([]Thought, 0),
		nextThoughtID:   1,
		KnownEntities:   make(map[string]string),
	}
}

// RecordTurn appends an interaction turn to subjective memory.
func (m *Memory) RecordTurn(turn TurnMemory) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Turns = append(m.Turns, turn)
}

// LearnEntities registers perceived entities into the actor's subjective catalog of existence.
func (m *Memory) LearnEntities(entities map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.KnownEntities == nil {
		m.KnownEntities = make(map[string]string)
	}
	for id, name := range entities {
		m.KnownEntities[id] = name
	}
}

// KnownEntitiesCopy returns a thread-safe snapshot of all entities known to the actor.
func (m *Memory) KnownEntitiesCopy() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copyMap := make(map[string]string, len(m.KnownEntities))
	for k, v := range m.KnownEntities {
		copyMap[k] = v
	}
	return copyMap
}

// AddThought seeds or inserts a thought directly into memory.
func (m *Memory) AddThought(t Thought) Thought {
	m.mu.Lock()
	defer m.mu.Unlock()

	if t.ID == "" {
		t.ID = fmt.Sprintf("th-%d", m.nextThoughtID)
		m.nextThoughtID++
	}
	if !t.Kind.Valid() {
		t.Kind = ThoughtBelief
	}
	if t.Salience <= 0 {
		t.Salience = 0.60
	} else if t.Salience > 1.0 {
		t.Salience = 1.0
	}

	// Register any linked entity IDs as known
	if m.KnownEntities == nil {
		m.KnownEntities = make(map[string]string)
	}
	for _, l := range t.Links {
		if _, ok := m.KnownEntities[l]; !ok {
			m.KnownEntities[l] = l
		}
	}

	m.Thoughts = append(m.Thoughts, t)
	return t
}

// ThoughtsSnapshot returns a safe deep copy of all current thoughts.
func (m *Memory) ThoughtsSnapshot() []Thought {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Thought, len(m.Thoughts))
	for i, t := range m.Thoughts {
		out[i] = t
		out[i].Links = append([]string(nil), t.Links...)
	}
	return out
}

// decayLocked applies passive salience decay over elapsed simulation ticks.
// Thoughts with salience dropping below 0.08 are forgotten unless reinforced.
func (m *Memory) decayLocked(currentTick int) {
	if m.decayedAt == 0 {
		m.decayedAt = currentTick
		return
	}
	elapsed := currentTick - m.decayedAt
	if elapsed <= 0 {
		return
	}
	m.decayedAt = currentTick

	factor := math.Pow(0.92, float64(elapsed))
	active := make([]Thought, 0, len(m.Thoughts))
	for _, t := range m.Thoughts {
		t.Salience *= factor
		if t.Salience >= 0.08 {
			active = append(active, t)
		}
	}
	m.Thoughts = active
}

// ApplyThoughtOps validates and executes structured thought mutations from the Thought Distiller.
func (m *Memory) ApplyThoughtOps(tick int, ops []ThoughtOp, perceived map[string]string) (applied []ThoughtOp, rejected []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.decayLocked(tick)

	// Combine currently perceived entities with previously known entities for link validation
	if m.KnownEntities == nil {
		m.KnownEntities = make(map[string]string)
	}
	for id, name := range perceived {
		m.KnownEntities[id] = name
	}

	for _, op := range ops {
		switch op.Op {
		case OpAdd:
			if strings.TrimSpace(op.Content) == "" {
				rejected = append(rejected, "add: empty content")
				continue
			}
			kind := op.Kind
			if !kind.Valid() {
				kind = ThoughtBelief
			}
			salience := 0.60
			if op.Salience != nil {
				salience = *op.Salience
				if salience < 0.05 {
					salience = 0.05
				} else if salience > 1.0 {
					salience = 1.0
				}
			}

			// Validate and filter links against known entities
			var validLinks []string
			for _, l := range op.Links {
				cleanL := strings.TrimSpace(l)
				if cleanL == "" {
					continue
				}
				if _, ok := m.KnownEntities[cleanL]; ok {
					validLinks = append(validLinks, cleanL)
				} else {
					rejected = append(rejected, fmt.Sprintf("add: link %q is unknown to actor (filtered out)", cleanL))
				}
			}

			// Evict lowest salience if over capacity
			if len(m.Thoughts) >= 24 {
				lowestIdx := 0
				lowestSal := m.Thoughts[0].Salience
				for i := 1; i < len(m.Thoughts); i++ {
					if m.Thoughts[i].Salience < lowestSal {
						lowestSal = m.Thoughts[i].Salience
						lowestIdx = i
					}
				}
				m.Thoughts = append(m.Thoughts[:lowestIdx], m.Thoughts[lowestIdx+1:]...)
			}

			newThought := Thought{
				ID:       fmt.Sprintf("th-%d", m.nextThoughtID),
				Kind:     kind,
				Content:  strings.TrimSpace(op.Content),
				Links:    validLinks,
				Salience: salience,
				Formed:   tick,
				Updated:  tick,
			}
			m.nextThoughtID++
			m.Thoughts = append(m.Thoughts, newThought)
			applied = append(applied, op)

		case OpUpdate:
			idx := m.findThoughtIndexLocked(op.ID)
			if idx == -1 {
				rejected = append(rejected, fmt.Sprintf("update: thought %q not found", op.ID))
				continue
			}
			if strings.TrimSpace(op.Content) != "" {
				m.Thoughts[idx].Content = strings.TrimSpace(op.Content)
			}
			if op.Kind.Valid() {
				m.Thoughts[idx].Kind = op.Kind
			}
			m.Thoughts[idx].Updated = tick
			applied = append(applied, op)

		case OpLink:
			idx := m.findThoughtIndexLocked(op.ID)
			if idx == -1 {
				rejected = append(rejected, fmt.Sprintf("link: thought %q not found", op.ID))
				continue
			}
			for _, l := range op.Links {
				cleanL := strings.TrimSpace(l)
				if cleanL == "" {
					continue
				}
				if _, ok := m.KnownEntities[cleanL]; !ok {
					rejected = append(rejected, fmt.Sprintf("link: entity %q is unknown to actor", cleanL))
					continue
				}
				// Deduplicate
				alreadyLinked := false
				for _, existing := range m.Thoughts[idx].Links {
					if existing == cleanL {
						alreadyLinked = true
						break
					}
				}
				if !alreadyLinked {
					m.Thoughts[idx].Links = append(m.Thoughts[idx].Links, cleanL)
				}
			}
			m.Thoughts[idx].Updated = tick
			applied = append(applied, op)

		case OpUnlink:
			idx := m.findThoughtIndexLocked(op.ID)
			if idx == -1 {
				rejected = append(rejected, fmt.Sprintf("unlink: thought %q not found", op.ID))
				continue
			}
			for _, l := range op.Links {
				cleanL := strings.TrimSpace(l)
				filtered := make([]string, 0, len(m.Thoughts[idx].Links))
				for _, existing := range m.Thoughts[idx].Links {
					if existing != cleanL {
						filtered = append(filtered, existing)
					}
				}
				m.Thoughts[idx].Links = filtered
			}
			m.Thoughts[idx].Updated = tick
			applied = append(applied, op)

		case OpSetSalience:
			idx := m.findThoughtIndexLocked(op.ID)
			if idx == -1 {
				rejected = append(rejected, fmt.Sprintf("set_salience: thought %q not found", op.ID))
				continue
			}
			if op.Salience == nil {
				rejected = append(rejected, "set_salience: missing salience value")
				continue
			}
			val := *op.Salience
			if val < 0.05 {
				val = 0.05
			} else if val > 1.0 {
				val = 1.0
			}
			m.Thoughts[idx].Salience = val
			m.Thoughts[idx].Updated = tick
			applied = append(applied, op)

		case OpDrop:
			idx := m.findThoughtIndexLocked(op.ID)
			if idx == -1 {
				rejected = append(rejected, fmt.Sprintf("drop: thought %q not found", op.ID))
				continue
			}
			m.Thoughts = append(m.Thoughts[:idx], m.Thoughts[idx+1:]...)
			applied = append(applied, op)

		default:
			rejected = append(rejected, fmt.Sprintf("unknown op %q", op.Op))
		}
	}

	return applied, rejected
}

func (m *Memory) findThoughtIndexLocked(id string) int {
	for i, t := range m.Thoughts {
		if t.ID == id {
			return i
		}
	}
	return -1
}

// SelectThoughts picks the most salient thoughts, boosting entries tied to entities
// currently relevant to the scene (e.g. co-present or in focus).
func (m *Memory) SelectThoughts(tick int, relevant map[string]float64, maxCount int) []Thought {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.Thoughts) == 0 {
		return nil
	}
	if maxCount <= 0 {
		maxCount = 8
	}

	m.decayLocked(tick)

	type scored struct {
		thought Thought
		score   float64
	}

	scoredList := make([]scored, len(m.Thoughts))
	for i, t := range m.Thoughts {
		score := t.Salience
		// Boost score if this thought is linked to entities currently in perception/focus
		for _, l := range t.Links {
			if boost, ok := relevant[l]; ok {
				score += boost * 0.35
			}
		}
		scoredList[i] = scored{thought: t, score: score}
	}

	sort.Slice(scoredList, func(i, j int) bool {
		if math.Abs(scoredList[i].score-scoredList[j].score) > 0.01 {
			return scoredList[i].score > scoredList[j].score
		}
		if scoredList[i].thought.Updated != scoredList[j].thought.Updated {
			return scoredList[i].thought.Updated > scoredList[j].thought.Updated
		}
		return scoredList[i].thought.ID < scoredList[j].thought.ID
	})

	limit := maxCount
	if len(scoredList) < limit {
		limit = len(scoredList)
	}

	selected := make([]Thought, limit)
	for i := 0; i < limit; i++ {
		selected[i] = scoredList[i].thought
	}
	return selected
}

// FormatThoughtContext formats active thoughts into a single grouped developer message.
func (m *Memory) FormatThoughtContext(tick int, relevant map[string]float64) string {
	selected := m.SelectThoughts(tick, relevant, 8)
	if len(selected) == 0 {
		return ""
	}

	var intentions, beliefs, suspicions, feelings []string
	for _, t := range selected {
		switch t.Kind {
		case ThoughtIntention:
			intentions = append(intentions, "  * "+t.Content)
		case ThoughtBelief:
			beliefs = append(beliefs, "  * "+t.Content)
		case ThoughtSuspicion:
			suspicions = append(suspicions, "  * "+t.Content)
		case ThoughtFeeling:
			feelings = append(feelings, "  * "+t.Content)
		}
	}

	var sb strings.Builder
	sb.WriteString("YOUR CURRENT STATE OF MIND (Private Thoughts & Priorities):\n")

	if len(intentions) > 0 {
		sb.WriteString("Active Intentions & Goals:\n")
		sb.WriteString(strings.Join(intentions, "\n"))
		sb.WriteString("\n")
	}
	if len(beliefs) > 0 {
		sb.WriteString("Current Beliefs & Assumptions:\n")
		sb.WriteString(strings.Join(beliefs, "\n"))
		sb.WriteString("\n")
	}
	if len(suspicions) > 0 {
		sb.WriteString("Suspicions & Concerns:\n")
		sb.WriteString(strings.Join(suspicions, "\n"))
		sb.WriteString("\n")
	}
	if len(feelings) > 0 {
		sb.WriteString("Physical & Emotional Sensations:\n")
		sb.WriteString(strings.Join(feelings, "\n"))
		sb.WriteString("\n")
	}

	return strings.TrimSpace(sb.String())
}
