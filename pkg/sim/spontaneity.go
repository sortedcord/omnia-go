package sim

import (
	"encoding/binary"
	"hash/fnv"
	"strings"
)

// ResolveTargets prefers exact names/IDs across the whole view, then an
// unambiguous partial name. Ambiguous partials are dropped rather than guessed.
func ResolveTargets(names []string, view []EntityRef) []EntityID {
	var result []EntityID
	seen := map[EntityID]bool{}
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		for _, exact := range []bool{true, false} {
			matches := map[EntityID]bool{}
			for _, p := range view {
				n := strings.ToLower(p.Name)
				if (exact && (name == n || name == strings.ToLower(string(p.ID)))) || (!exact && strings.Contains(n, name)) {
					matches[p.ID] = true
				}
			}
			if len(matches) == 0 {
				continue
			}
			if len(matches) == 1 {
				for id := range matches {
					if !seen[id] {
						seen[id] = true
						result = append(result, id)
					}
				}
			}
			break
		}
	}
	return result
}

// deterministicRoll is independent of map iteration, goroutine completion, and
// the number of other actors. Salts give luck and jitter independent streams.
func deterministicRoll(seed int64, tick int, actorID EntityID, salt string) float64 {
	h := fnv.New64a()
	var data [16]byte
	binary.LittleEndian.PutUint64(data[:8], uint64(seed))
	binary.LittleEndian.PutUint64(data[8:], uint64(tick))
	h.Write(data[:])
	h.Write([]byte(salt))
	h.Write([]byte{0})
	h.Write([]byte(actorID))
	return float64(h.Sum64()>>11) / (1 << 53)
}

// ComputeSpontaneity is advisory only; targets never gate perception or turns.
func (s *Simulation) ComputeSpontaneity(id EntityID, tick int, event *Event) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entity := s.Entities[id]
	if entity == nil {
		return 0
	}
	targeted, attention, proximity := 0.0, 0.2, 1.0
	if event != nil {
		for _, target := range event.Targets {
			if target == id {
				targeted = 1
				break
			}
		}
		for _, source := range event.Sources {
			attention = max(attention, 0.2+s.FocusMap[id][source])
		}
		if event.SourceID != "" {
			attention = max(attention, 0.2+s.FocusMap[id][event.SourceID])
		}
		if event.LocationID != "" {
			proximity = 0
			if loc, ok := s.whereLocked(id); ok {
				if loc.ID == event.LocationID {
					proximity = 1
				} else if s.portalBetweenLocked(loc.ID, event.LocationID) {
					proximity = 0.5
				}
			}
		}
	}
	readiness := 0.5
	if entity.Attributes != nil {
		switch value, _ := entity.Attributes.Get("arousal"); value {
		case "low":
			readiness = 0.2
		case "high":
			readiness = 0.85
		}
	}
	return 0.25*targeted + 0.30*min(attention, 1) + 0.20*readiness + 0.10*proximity + 0.15*deterministicRoll(s.Seed, tick, id, "spontaneity")
}
