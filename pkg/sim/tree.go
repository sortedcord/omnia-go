package sim

import "fmt"

// AddEntity registers a new entity into the simulation tree.
func (s *Simulation) AddEntity(e *Entity) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e.ID == "" {
		return fmt.Errorf("tree: entity ID cannot be empty")
	}
	if _, exists := s.Entities[e.ID]; exists {
		return fmt.Errorf("tree: entity %q already exists", e.ID)
	}

	if e.Parent != "" {
		parent, ok := s.Entities[e.Parent]
		if !ok {
			return fmt.Errorf("tree: parent entity %q not found", e.Parent)
		}
		parent.Children = append(parent.Children, e.ID)
		parent.Revision++
	}

	s.Entities[e.ID] = e
	return nil
}

// GetEntity retrieves an entity by its ID.
func (s *Simulation) GetEntity(id EntityID) (*Entity, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, ok := s.Entities[id]
	return e, ok
}

// RemoveEntity removes an entity and cascades deletion to all descendants.
func (s *Simulation) RemoveEntity(id EntityID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.Entities[id]
	if !ok {
		return fmt.Errorf("tree: entity %q not found", id)
	}

	// Remove from parent's children list
	if e.Parent != "" {
		if parent, ok := s.Entities[e.Parent]; ok {
			filtered := make([]EntityID, 0, len(parent.Children))
			for _, childID := range parent.Children {
				if childID != id {
					filtered = append(filtered, childID)
				}
			}
			parent.Children = filtered
			parent.Revision++
		}
	}

	// Recursively delete this entity and all its descendants
	var deleteSubtree func(currID EntityID)
	deleteSubtree = func(currID EntityID) {
		curr, ok := s.Entities[currID]
		if !ok {
			return
		}
		for _, childID := range curr.Children {
			deleteSubtree(childID)
		}
		delete(s.Entities, currID)
		delete(s.inbox, currID)
		for _, fmap := range s.FocusMap {
			delete(fmap, currID)
		}
		delete(s.FocusMap, currID)
	}

	deleteSubtree(id)
	return nil
}

// Move changes an entity's parent and link relationship in the containment hierarchy.
// Automatically prevents containment cycles.
func (s *Simulation) Move(id EntityID, newParent EntityID, link Link) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.moveLocked(id, newParent, link)
}

// Where returns the nearest enterable space containing an entity (e.g. room or vehicle cabin).
func (s *Simulation) Where(id EntityID) (*Entity, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.whereLocked(id)
}

// Ancestors returns all parent entities walking upwards to the root.
func (s *Simulation) Ancestors(id EntityID) []*Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ancestorsLocked(id)
}

// Descendants recursively returns all entities contained inside this entity's subtree.
func (s *Simulation) Descendants(id EntityID) []*Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Entity
	var collect func(currID EntityID)

	collect = func(currID EntityID) {
		e, ok := s.Entities[currID]
		if !ok {
			return
		}
		for _, childID := range e.Children {
			if child, ok := s.Entities[childID]; ok {
				result = append(result, child)
				collect(childID)
			}
		}
	}

	collect(id)
	return result
}

// Carried returns all entities currently worn, held, or contained inside an actor's equipment.
func (s *Simulation) Carried(actorID EntityID) []*Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Entity
	var collect func(currID EntityID)

	collect = func(currID EntityID) {
		e, ok := s.Entities[currID]
		if !ok {
			return
		}
		for _, childID := range e.Children {
			if child, ok := s.Entities[childID]; ok {
				result = append(result, child)
				collect(childID)
			}
		}
	}

	actorEntity, ok := s.Entities[actorID]
	if !ok {
		return nil
	}

	for _, childID := range actorEntity.Children {
		child, ok := s.Entities[childID]
		if !ok {
			continue
		}
		if child.Link.Relation == RelHeld || child.Link.Relation == RelWorn {
			result = append(result, child)
			collect(childID)
		}
	}
	return result
}

// ChildrenOf returns immediate children entities of a given parent entity.
func (s *Simulation) ChildrenOf(parentID EntityID) []*Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	parent, ok := s.Entities[parentID]
	if !ok {
		return nil
	}

	result := make([]*Entity, 0, len(parent.Children))
	for _, childID := range parent.Children {
		if child, ok := s.Entities[childID]; ok {
			result = append(result, child)
		}
	}
	return result
}

// Roommates returns all other entities that share the same immediate enterable space as the given entity.
func (s *Simulation) Roommates(id EntityID) []*Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	space, ok := s.whereLocked(id)
	if !ok || space == nil {
		return nil
	}

	var result []*Entity
	for _, childID := range space.Children {
		if childID != id {
			if child, ok := s.Entities[childID]; ok {
				result = append(result, child)
			}
		}
	}
	return result
}

// whereLocked helper without acquiring mutex (caller must hold RLock).
func (s *Simulation) whereLocked(id EntityID) (*Entity, bool) {
	e, ok := s.Entities[id]
	if !ok {
		return nil, false
	}
	if e.Space != nil && e.Space.Enterable {
		return e, true
	}
	curr := e
	for curr.Parent != "" {
		parent, ok := s.Entities[curr.Parent]
		if !ok {
			break
		}
		if parent.Space != nil && parent.Space.Enterable {
			return parent, true
		}
		curr = parent
	}
	return nil, false
}
