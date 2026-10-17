package sim

import (
	"fmt"
	"sort"
)

// SceneID is the unique identifier for a dynamically partitioned scene.
type SceneID string

// Scene represents a group of actors and enterable spaces that can mutually perceive
// and interact with each other in real-time.
type Scene struct {
	ID              SceneID    `json:"id"`
	CreatedTick     int        `json:"created_tick"`
	LastTick        int        `json:"last_tick"`
	SpaceIDs        []EntityID `json:"space_ids"`
	ActorIDs        []EntityID `json:"actor_ids"`
	ContestedEntity EntityID   `json:"contested_entity,omitempty"` // Entity causing an inter-scene bond
	BondedTo        SceneID    `json:"bonded_to,omitempty"`        // If bonded, the parent scene it merges with
}

// BuildScenes partitions the world into dynamic scenes based on spatial connectivity
// and active inter-scene conflict bonds.
func (s *Simulation) BuildScenes(currentTick int) map[SceneID]*Scene {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buildScenesLocked(currentTick)
}

func (s *Simulation) buildScenesLocked(currentTick int) map[SceneID]*Scene {
	// 1. Identify all enterable spaces in the world
	var enterableSpaces []EntityID
	for id, e := range s.Entities {
		if e.Space != nil && e.Space.Enterable {
			enterableSpaces = append(enterableSpaces, id)
		}
	}
	sort.Slice(enterableSpaces, func(i, j int) bool {
		return enterableSpaces[i] < enterableSpaces[j]
	})

	// 2. Build adjacency graph of spaces connected by open or transparent portals
	adj := make(map[EntityID][]EntityID)
	for _, spaceID := range enterableSpaces {
		space := s.Entities[spaceID]
		for _, childID := range space.Children {
			child, ok := s.Entities[childID]
			if !ok || child.Portal == nil {
				continue
			}
			targetID := child.Portal.Target
			targetSpace, ok := s.Entities[targetID]
			if !ok || targetSpace.Space == nil || !targetSpace.Space.Enterable {
				continue
			}

			// Open or transparent portal connects spaces
			if !child.Portal.Closed || (child.Space != nil && child.Space.Transparent) {
				adj[spaceID] = append(adj[spaceID], targetID)
				adj[targetID] = append(adj[targetID], spaceID)
			}
		}
	}

	// 3. Find connected components (BFS / DFS)
	visited := make(map[EntityID]bool)
	var components [][]EntityID

	for _, spaceID := range enterableSpaces {
		if visited[spaceID] {
			continue
		}
		var comp []EntityID
		queue := []EntityID{spaceID}
		visited[spaceID] = true

		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]
			comp = append(comp, curr)

			for _, neighbor := range adj[curr] {
				if !visited[neighbor] {
					visited[neighbor] = true
					queue = append(queue, neighbor)
				}
			}
		}

		sort.Slice(comp, func(i, j int) bool {
			return comp[i] < comp[j]
		})
		components = append(components, comp)
	}

	// 4. Map actors to each component
	type rawComponent struct {
		spaces []EntityID
		actors []EntityID
	}
	rawComps := make([]rawComponent, len(components))

	for i, comp := range components {
		spaceSet := make(map[EntityID]bool)
		for _, sp := range comp {
			spaceSet[sp] = true
		}

		var actorsInComp []EntityID
		for id, e := range s.Entities {
			if e.Mind == nil {
				continue
			}
			loc, ok := s.whereLocked(id)
			if ok && loc != nil && spaceSet[loc.ID] {
				actorsInComp = append(actorsInComp, id)
			}
		}

		sort.Slice(actorsInComp, func(i, j int) bool {
			return actorsInComp[i] < actorsInComp[j]
		})

		rawComps[i] = rawComponent{
			spaces: comp,
			actors: actorsInComp,
		}
	}

	// 5. Match with previous scenes to preserve SceneIDs across ticks
	newScenes := make(map[SceneID]*Scene)
	usedPrevIDs := make(map[SceneID]bool)

	for _, comp := range rawComps {
		// Find overlapping previous scenes
		prevMatches := make(map[SceneID]int)
		for _, aID := range comp.actors {
			for prevID, prevScene := range s.Scenes {
				for _, prevActor := range prevScene.ActorIDs {
					if prevActor == aID {
						prevMatches[prevID]++
					}
				}
			}
		}

		var bestID SceneID
		bestOverlap := -1
		oldestTick := 999999

		// On merge: oldest scene ID survives
		// On split: largest group retains ID
		for prevID, count := range prevMatches {
			if usedPrevIDs[prevID] {
				continue
			}
			prevScene := s.Scenes[prevID]
			if count > bestOverlap || (count == bestOverlap && prevScene.CreatedTick < oldestTick) {
				bestID = prevID
				bestOverlap = count
				oldestTick = prevScene.CreatedTick
			}
		}

		if bestID == "" {
			// Generate new SceneID
			s.nextSceneID++
			bestID = SceneID(fmt.Sprintf("scene-%d", s.nextSceneID))
			oldestTick = currentTick
		}
		usedPrevIDs[bestID] = true

		newScenes[bestID] = &Scene{
			ID:          bestID,
			CreatedTick: oldestTick,
			LastTick:    currentTick,
			SpaceIDs:    comp.spaces,
			ActorIDs:    comp.actors,
		}
	}

	// 6. Handle active conflict bonds: merge newer scene into older scene
	for bondTarget, bondSource := range s.SceneBonds {
		targetScene, hasTarget := newScenes[bondTarget]
		sourceScene, hasSource := newScenes[bondSource]

		if hasTarget && hasSource && bondTarget != bondSource {
			// Absorb source into target (older scene)
			targetScene.SpaceIDs = append(targetScene.SpaceIDs, sourceScene.SpaceIDs...)
			targetScene.ActorIDs = append(targetScene.ActorIDs, sourceScene.ActorIDs...)
			targetScene.BondedTo = bondTarget
			delete(newScenes, bondSource)
		}
	}

	s.Scenes = newScenes
	return newScenes
}

// SceneForActor returns the Scene an actor currently belongs to.
func (s *Simulation) SceneForActor(actorID EntityID) (*Scene, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sceneForActorLocked(actorID)
}

func (s *Simulation) sceneForActorLocked(actorID EntityID) (*Scene, bool) {
	for _, sc := range s.Scenes {
		for _, aID := range sc.ActorIDs {
			if aID == actorID {
				return sc, true
			}
		}
	}
	return nil, false
}
