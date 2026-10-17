package sim

import (
	"hash/fnv"
	"strings"
)

// ResolveTargets maps an actor's declared target names to perceivable entity IDs.
// Any target that cannot be resolved against what the actor can perceive is dropped.
func ResolveTargets(targetNames []string, perceivable []EntityRef) []EntityID {
	if len(targetNames) == 0 || len(perceivable) == 0 {
		return nil
	}

	var resolved []EntityID
	seen := make(map[EntityID]bool)

	for _, rawTarget := range targetNames {
		clean := strings.TrimSpace(strings.ToLower(rawTarget))
		if clean == "" {
			continue
		}

		for _, p := range perceivable {
			pName := strings.ToLower(p.Name)
			pID := strings.ToLower(string(p.ID))

			// Match by exact ID, exact Name, or substring
			if clean == pID || clean == pName || strings.Contains(pName, clean) {
				if !seen[p.ID] {
					seen[p.ID] = true
					resolved = append(resolved, p.ID)
				}
				break
			}
		}
	}

	return resolved
}

// ComputeSpontaneity calculates an actor's readiness and urge to react to an event.
// Formula: score = 0.25*targeted + 0.30*attention + 0.20*readiness + 0.10*proximity + 0.15*jitter
func (s *Simulation) ComputeSpontaneity(actorID EntityID, currentTick int, triggerEvent *Event) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	actorEntity, ok := s.Entities[actorID]
	if !ok {
		return 0.0
	}

	// 1. Targeted Score (0.25): Was this actor specifically addressed?
	targetedScore := 0.0
	if triggerEvent != nil {
		lowerName := strings.ToLower(actorEntity.Name)
		lowerID := strings.ToLower(string(actorID))
		descLower := strings.ToLower(triggerEvent.Description)

		if strings.Contains(descLower, lowerName) || strings.Contains(descLower, lowerID) {
			targetedScore = 1.0
		}
	}

	// 2. Attention Score (0.30): Actor's current focus on the trigger entity
	attentionScore := 0.20 // default baseline
	if triggerEvent != nil && triggerEvent.SourceID != "" {
		if fmap, exists := s.FocusMap[actorID]; exists {
			if boost, has := fmap[triggerEvent.SourceID]; has {
				attentionScore += boost
			}
		}
	}
	if attentionScore > 1.0 {
		attentionScore = 1.0
	}

	// 3. Readiness Score (0.20): Derived from arousal attribute
	readinessScore := 0.50
	if actorEntity.Mind != nil {
		if arousal, ok := actorEntity.Mind.Attributes.Get("arousal"); ok {
			switch strings.ToLower(arousal) {
			case "high":
				readinessScore = 0.85
			case "low":
				readinessScore = 0.20
			default:
				readinessScore = 0.50
			}
		}
	}

	// 4. Proximity Score (0.10): Spatial distance to the event
	proximityScore := 0.0
	if triggerEvent != nil && triggerEvent.LocationID != "" {
		actorLoc, ok := s.whereLocked(actorID)
		if ok && actorLoc != nil {
			if actorLoc.ID == triggerEvent.LocationID {
				proximityScore = 1.0
			} else if s.portalBetweenLocked(actorLoc.ID, triggerEvent.LocationID) {
				proximityScore = 0.5
			}
		}
	} else {
		proximityScore = 1.0
	}

	// 5. Deterministic Jitter (0.15): FNV-1a hash of (seed, tick, actorID)
	h := fnv.New64a()
	_, _ = h.Write([]byte(string(actorID)))
	hashVal := (h.Sum64() + uint64(s.Seed) + uint64(currentTick*31)) % 1000
	jitterScore := float64(hashVal) / 1000.0

	// Weighted sum
	total := (0.25 * targetedScore) +
		(0.30 * attentionScore) +
		(0.20 * readinessScore) +
		(0.10 * proximityScore) +
		(0.15 * jitterScore)

	if total > 1.0 {
		total = 1.0
	}
	if total < 0.0 {
		total = 0.0
	}

	return total
}
