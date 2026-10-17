package sim

import (
	"fmt"
	"sort"
	"strings"

	"omnia.adityagupta.dev/pkg/actor"
)

// applySceneEffectsLocked executes the Arbiter's requested mutations for a scene.
// It detects cross-scene conflicts: if a newer scene tries to mutate an entity already
// modified by an older scene in the same tick, the effect is rejected and an inter-scene
// conflict bond is established so the older scene absorbs the newer one next tick.
// Caller MUST hold s.mu.Lock().
func (s *Simulation) applySceneEffectsLocked(scene *Scene, ruling *SceneRuling, tickMutated map[EntityID]SceneID) {
	sceneSpaces := make(map[EntityID]bool)
	for _, spID := range scene.SpaceIDs {
		sceneSpaces[spID] = true
	}

	for _, eff := range ruling.Effects {
		// Conflict Detection: check if entity was already mutated this tick by an older scene
		if olderSceneID, alreadyMutated := tickMutated[eff.EntityID]; alreadyMutated && olderSceneID != scene.ID {
			ruling.RejectedEffects = append(ruling.RejectedEffects,
				fmt.Sprintf("conflict: entity %q was already modified by older scene %q; bonding scenes for next tick", eff.EntityID, olderSceneID))

			// Bond scenes for next tick: older scene absorbs newer scene
			if s.SceneBonds == nil {
				s.SceneBonds = make(map[SceneID]SceneID)
			}
			s.SceneBonds[olderSceneID] = scene.ID
			s.ContestedEntities[eff.EntityID] = true
			continue
		}

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

			// Reachability check
			actorLoc, _ := s.whereLocked(eff.EntityID)
			destSpace, _ := s.whereLocked(eff.ParentID)

			actorSpaceID := EntityID("")
			if actorLoc != nil {
				actorSpaceID = actorLoc.ID
			}
			destSpaceID := EntityID("")
			if destSpace != nil {
				destSpaceID = destSpace.ID
			}

			if actorSpaceID != "" && destSpaceID != "" && actorSpaceID != destSpaceID {
				// Moving between rooms requires an open portal
				if !s.portalBetweenLocked(actorSpaceID, destSpaceID) {
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

			tickMutated[eff.EntityID] = scene.ID
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
			tickMutated[eff.EntityID] = scene.ID
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
			tickMutated[eff.EntityID] = scene.ID
			ruling.AppliedEffects = append(ruling.AppliedEffects, eff)

		case EffectSetAttribute:
			targetEntity, ok := s.Entities[eff.EntityID]
			if !ok || targetEntity.Mind == nil {
				ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("set_attribute: %q has no mind component", eff.EntityID))
				continue
			}
			if eff.Key == "" {
				ruling.RejectedEffects = append(ruling.RejectedEffects, "set_attribute: key cannot be empty")
				continue
			}
			if targetEntity.Mind.Attributes == nil {
				targetEntity.Mind.Attributes = actor.NewAttributes()
			}
			if err := targetEntity.Mind.Attributes.Set(eff.Key, eff.Value); err != nil {
				ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("set_attribute: %v", err))
				continue
			}
			targetEntity.Revision++
			tickMutated[eff.EntityID] = scene.ID
			ruling.AppliedEffects = append(ruling.AppliedEffects, eff)

		case EffectRemoveAttribute:
			targetEntity, ok := s.Entities[eff.EntityID]
			if !ok || targetEntity.Mind == nil {
				ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("remove_attribute: %q has no mind component", eff.EntityID))
				continue
			}
			if err := targetEntity.Mind.Attributes.Remove(eff.Key); err != nil {
				ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("remove_attribute: %v", err))
				continue
			}
			targetEntity.Revision++
			tickMutated[eff.EntityID] = scene.ID
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
				tickMutated[eff.EntityID] = scene.ID
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
			tickMutated[eff.EntityID] = scene.ID
			ruling.AppliedEffects = append(ruling.AppliedEffects, eff)

		default:
			ruling.RejectedEffects = append(ruling.RejectedEffects, fmt.Sprintf("unknown effect type: %q", eff.Type))
		}
	}
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

// renderObjectiveSpacesLocked builds an unfiltered, exhaustive view of all spaces in a scene
// for the Arbiter.
func (s *Simulation) renderObjectiveSpacesLocked(spaceIDs []EntityID) string {
	var sb strings.Builder

	for i, spID := range spaceIDs {
		if i > 0 {
			sb.WriteString("\n--------------------------------------------------\n")
		}
		space, ok := s.Entities[spID]
		if !ok {
			continue
		}

		sb.WriteString(fmt.Sprintf("SPACE: %s [ID: %s]\n", space.Name, space.ID))
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

		sb.WriteString("\nENTITIES IN THIS SPACE (Hierarchy):\n")
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
				// Render core attributes
				if e.Mind.Attributes != nil {
					var attrPairs []string
					for k, v := range e.Mind.Attributes {
						attrPairs = append(attrPairs, fmt.Sprintf("%s=%s", k, v.Value))
					}
					sort.Strings(attrPairs)
					extra += fmt.Sprintf(" {attrs: %s}", strings.Join(attrPairs, ", "))
				}
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
	}

	return strings.TrimSpace(sb.String())
}

// recentSceneEventsLocked pulls recent events occurring in any of the scene's spaces.
func (s *Simulation) recentSceneEventsLocked(spaceIDs []EntityID, limit int) []string {
	spaceSet := make(map[EntityID]bool)
	for _, id := range spaceIDs {
		spaceSet[id] = true
	}

	var matched []string
	for i := len(s.EventLog) - 1; i >= 0 && len(matched) < limit; i-- {
		ev := s.EventLog[i]
		if spaceSet[ev.LocationID] {
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

// recentGlobalEventsLocked pulls the last N events across the entire simulation.
func (s *Simulation) recentGlobalEventsLocked(limit int) []string {
	start := len(s.EventLog) - limit
	if start < 0 {
		start = 0
	}
	var matched []string
	for i := start; i < len(s.EventLog); i++ {
		ev := s.EventLog[i]
		desc := ev.Description
		if ev.Observed != "" {
			desc = ev.Observed
		}
		matched = append(matched, fmt.Sprintf("[Tick %d | Loc: %s] %s", ev.Tick, ev.LocationID, desc))
	}
	return matched
}
