package sim

import (
	"fmt"
	"sort"
	"strings"
)

// applyEffectsLocked executes the Arbiter's requested mutations against the simulation state.
// Caller MUST hold s.mu.Lock().
func (s *Simulation) applyEffectsLocked(actingActorID EntityID, ruling *Ruling) {
	actingActor, ok := s.Entities[actingActorID]
	if !ok {
		return
	}

	actorSpace, _ := s.whereLocked(actingActorID)
	actorSpaceID := EntityID("")
	if actorSpace != nil {
		actorSpaceID = actorSpace.ID
	}

	for _, eff := range ruling.Effects {
		switch eff.Type {
		case EffectMove:
			targetEntity, ok := s.Entities[eff.EntityID]
			if !ok {
				ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("move: entity %q does not exist", eff.EntityID))
				continue
			}

			// Do not allow moving root spaces
			if targetEntity.Space != nil && targetEntity.Space.Enterable && targetEntity.Parent == "" {
				ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("move: cannot move top-level room %q", eff.EntityID))
				continue
			}

			// Validate target parent exists
			if _, ok := s.Entities[eff.ParentID]; !ok {
				ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("move: parent %q does not exist", eff.ParentID))
				continue
			}

			// Validate relation
			rel := eff.Relation
			if rel == "" {
				rel = RelIn
			}
			switch rel {
			case RelIn, RelOn, RelUnder, RelBehind, RelWorn, RelHeld, RelSeated, RelContained:
				// Valid
			default:
				ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("move: invalid relation %q", rel))
				continue
			}

			// Check reachability
			// An actor can move items within their space, or move themselves through an open portal to a connected space
			destSpace, _ := s.whereLocked(eff.ParentID)
			destSpaceID := EntityID("")
			if destSpace != nil {
				destSpaceID = destSpace.ID
			}

			isCrossSpace := actorSpaceID != "" && destSpaceID != "" && actorSpaceID != destSpaceID
			if isCrossSpace {
				// Cross-space moves are only allowed if the moving entity is the actor itself moving through an open portal
				if eff.EntityID != actingActorID {
					ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("move: cannot manipulate %q into a different room (%q)", eff.EntityID, destSpaceID))
					continue
				}
				// Verify portal exists and is open
				portalOpen := s.portalBetweenLocked(actorSpaceID, destSpaceID)
				if !portalOpen {
					ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("move: no open portal between %q and %q", actorSpaceID, destSpaceID))
					continue
				}
			}

			link := Link{
				Relation:  rel,
				Placement: eff.Placement,
				Concealed: targetEntity.Link.Concealed,
			}

			if err := s.moveLocked(eff.EntityID, eff.ParentID, link); err != nil {
				ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("move failed: %v", err))
				continue
			}

			ruling.AppliedEffects = append(ruling.AppliedEffects, eff)

		case EffectSetCondition:
			spaceEntity, ok := s.Entities[eff.EntityID]
			if !ok || spaceEntity.Space == nil {
				ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("set_condition: %q has no space component", eff.EntityID))
				continue
			}
			if eff.Key == "" {
				ruling.RejectedEffects = append(ruling.RejectedEffects, "set_condition: key cannot be empty")
				continue
			}

			if spaceEntity.Space.Conditions == nil {
				spaceEntity.Space.Conditions = make(map[string]string)
			}

			if eff.Value == "" {
				delete(spaceEntity.Space.Conditions, eff.Key)
			} else {
				spaceEntity.Space.Conditions[eff.Key] = eff.Value
			}
			spaceEntity.Revision++
			ruling.AppliedEffects = append(ruling.AppliedEffects, eff)

		case EffectSetPortal:
			portalEntity, ok := s.Entities[eff.EntityID]
			if !ok || portalEntity.Portal == nil {
				ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("set_portal: %q is not a portal", eff.EntityID))
				continue
			}

			if eff.Closed != nil {
				portalEntity.Portal.Closed = *eff.Closed
			}
			if eff.Locked != nil {
				portalEntity.Portal.Locked = *eff.Locked
				if *eff.Locked {
					portalEntity.Portal.Closed = true
				}
			}
			portalEntity.Revision++
			ruling.AppliedEffects = append(ruling.AppliedEffects, eff)

		case EffectSetVital:
			vitalEntity, ok := s.Entities[eff.EntityID]
			if !ok || vitalEntity.Mind == nil {
				ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("set_vital: %q has no mind component", eff.EntityID))
				continue
			}
			if eff.Key == "" {
				ruling.RejectedEffects = append(ruling.RejectedEffects, "set_vital: key cannot be empty")
				continue
			}

			if vitalEntity.Mind.Status.Vitals == nil {
				vitalEntity.Mind.Status.Vitals = make(map[string]string)
			}

			if eff.Value == "" {
				delete(vitalEntity.Mind.Status.Vitals, eff.Key)
			} else {
				vitalEntity.Mind.Status.Vitals[eff.Key] = eff.Value
			}
			vitalEntity.Revision++
			ruling.AppliedEffects = append(ruling.AppliedEffects, eff)

		case EffectSetOpen:
			openEntity, ok := s.Entities[eff.EntityID]
			if !ok || openEntity.Space == nil {
				ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("set_open: %q has no space component", eff.EntityID))
				continue
			}
			if eff.Open != nil {
				openEntity.Space.Open = *eff.Open
				openEntity.Revision++
				ruling.AppliedEffects = append(ruling.AppliedEffects, eff)
			}

		case EffectReveal:
			revealEntity, ok := s.Entities[eff.EntityID]
			if !ok {
				ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("reveal: entity %q not found", eff.EntityID))
				continue
			}
			revealEntity.Link.Concealed = false
			revealEntity.Revision++
			ruling.AppliedEffects = append(ruling.AppliedEffects, eff)

		default:
			ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("unknown effect type: %q", eff.Type))
		}
	}

	_ = actingActor
}

// moveLocked handles entity reparenting without acquiring mutex (caller must hold Lock).
func (s *Simulation) moveLocked(id EntityID, newParent EntityID, link Link) error {
	e, ok := s.Entities[id]
	if !ok {
		return fmt.Errorf("entity %q not found", id)
	}

	if newParent != "" {
		parent, ok := s.Entities[newParent]
		if !ok {
			return fmt.Errorf("target parent %q not found", newParent)
		}

		// Prevent containment cycle: newParent cannot be a descendant of id
		curr := parent
		for curr != nil {
			if curr.ID == id {
				return fmt.Errorf("containment cycle: %q is a descendant of %q", newParent, id)
			}
			if curr.Parent == "" {
				break
			}
			curr = s.Entities[curr.Parent]
		}

		// Remove from old parent
		if e.Parent != "" {
			if oldParent, ok := s.Entities[e.Parent]; ok {
				filtered := make([]EntityID, 0, len(oldParent.Children))
				for _, childID := range oldParent.Children {
					if childID != id {
						filtered = append(filtered, childID)
					}
				}
				oldParent.Children = filtered
				oldParent.Revision++
			}
		}

		e.Parent = newParent
		e.Link = link
		e.Revision++

		parent.Children = append(parent.Children, id)
		parent.Revision++
	} else {
		// Detach to root
		if e.Parent != "" {
			if oldParent, ok := s.Entities[e.Parent]; ok {
				filtered := make([]EntityID, 0, len(oldParent.Children))
				for _, childID := range oldParent.Children {
					if childID != id {
						filtered = append(filtered, childID)
					}
				}
				oldParent.Children = filtered
				oldParent.Revision++
			}
		}
		e.Parent = ""
		e.Link = link
		e.Revision++
	}

	return nil
}

// portalBetweenLocked checks if an open portal connects spaceA and spaceB in either direction.
func (s *Simulation) portalBetweenLocked(spaceA, spaceB EntityID) bool {
	for _, e := range s.Entities {
		if e.Portal == nil {
			continue
		}
		fromSpace, _ := s.whereLocked(e.ID)
		fromID := EntityID("")
		if fromSpace != nil {
			fromID = fromSpace.ID
		}
		toSpace, _ := s.whereLocked(e.Portal.Target)
		toID := e.Portal.Target
		if toSpace != nil {
			toID = toSpace.ID
		}

		if (fromID == spaceA && toID == spaceB) || (fromID == spaceB && toID == spaceA) {
			if !e.Portal.Closed && !e.Portal.Locked {
				return true
			}
		}
	}
	return false
}

// renderObjectiveLocked builds an unfiltered, exhaustive view of a room and all entities
// inside it (including concealed objects and closed sub-containers) for the Arbiter.
func (s *Simulation) renderObjectiveLocked(spaceID EntityID) string {
	space, ok := s.Entities[spaceID]
	if !ok {
		return "(unknown space)"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ROOM / SPACE: %s [ID: %s]\n", space.Name, space.ID))
	if space.Description != "" {
		sb.WriteString(fmt.Sprintf("Description: %s\n", space.Description))
	}
	if space.Space != nil && len(space.Space.Conditions) > 0 {
		sb.WriteString("Conditions:\n")
		var keys []string
		for k := range space.Space.Conditions {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", k, space.Space.Conditions[k]))
		}
	}

	sb.WriteString("\nENTITIES IN THIS SPACE (Full Containment Hierarchy):\n")
	var renderSubtree func(e *Entity, depth int)
	renderSubtree = func(e *Entity, depth int) {
		indent := strings.Repeat("  ", depth)
		rel := string(e.Link.Relation)
		if e.Link.Placement != "" {
			rel = fmt.Sprintf("%s, %s", e.Link.Relation, e.Link.Placement)
		}
		if e.Link.Concealed {
			rel += ", CONCEALED"
		}

		extra := ""
		if e.Space != nil {
			status := "open"
			if !e.Space.Open {
				status = "closed"
			}
			extra += fmt.Sprintf(" (container space: %s)", status)
		}
		if e.Portal != nil {
			status := "open"
			if e.Portal.Closed {
				status = "closed"
			}
			if e.Portal.Locked {
				status = "locked"
			}
			extra += fmt.Sprintf(" (portal -> %s [%s])", e.Portal.Target, status)
		}
		if e.Mind != nil {
			extra += " [ACTOR]"
		}

		sb.WriteString(fmt.Sprintf("%s- %s [ID: %s] (%s)%s\n", indent, e.Name, e.ID, rel, extra))
		if e.Description != "" {
			sb.WriteString(fmt.Sprintf("%s    Description: %s\n", indent, e.Description))
		}

		for _, childID := range e.Children {
			if child, ok := s.Entities[childID]; ok {
				renderSubtree(child, depth+1)
			}
		}
	}

	for _, childID := range space.Children {
		if child, ok := s.Entities[childID]; ok {
			renderSubtree(child, 0)
		}
	}

	return strings.TrimSpace(sb.String())
}

// recentEventsLocked pulls the last N events that took place in a given location.
func (s *Simulation) recentEventsLocked(locID EntityID, limit int) []string {
	var matched []string
	for i := len(s.EventLog) - 1; i >= 0 && len(matched) < limit; i-- {
		ev := s.EventLog[i]
		if ev.LocationID == locID {
			desc := ev.Description
			if ev.Observed != "" {
				desc = ev.Observed
			}
			matched = append(matched, fmt.Sprintf("[Tick %d | %s] %s", ev.Tick, ev.Kind, desc))
		}
	}

	// Reverse to restore chronological order
	for i, j := 0, len(matched)-1; i < j; i, j = i+1, j-1 {
		matched[i], matched[j] = matched[j], matched[i]
	}
	return matched
}
