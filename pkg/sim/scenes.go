package sim

import (
	"fmt"
	"slices"
	"sort"
)

type SceneID string

// Scene is a connected interaction group, not a controller execution order.
type Scene struct {
	ID          SceneID    `json:"id"`
	CreatedTick int        `json:"created_tick"`
	LastTick    int        `json:"last_tick"`
	SpaceIDs    []EntityID `json:"space_ids"`
	ActorIDs    []EntityID `json:"actor_ids"`
	sequence    int
	memberSince map[EntityID]int
}

// Bonds track actors, not scene IDs: scene IDs can disappear during a merge.
type sceneBond struct {
	Entity      EntityID
	Left, Right []EntityID
	LastTick    int
}

func olderScene(a, b *Scene) bool {
	if a.CreatedTick != b.CreatedTick {
		return a.CreatedTick < b.CreatedTick
	}
	return a.sequence < b.sequence
}

func (s *Simulation) BuildScenes(tick int) map[SceneID]*Scene {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buildScenesLocked(tick)
}

func (s *Simulation) buildScenesLocked(tick int) map[SceneID]*Scene {
	var spaces []EntityID
	adjacency := map[EntityID][]EntityID{}
	connect := func(a, b EntityID) { adjacency[a] = append(adjacency[a], b); adjacency[b] = append(adjacency[b], a) }
	for id, e := range s.Entities {
		if e.Space != nil && e.Space.Enterable {
			spaces = append(spaces, id)
		}
	}
	slices.Sort(spaces)
	for _, id := range spaces {
		for _, childID := range s.Entities[id].Children {
			child := s.Entities[childID]
			if child == nil || child.Portal == nil {
				continue
			}
			target := s.Entities[child.Portal.Target]
			if target == nil || target.Space == nil || !target.Space.Enterable {
				continue
			}
			if !child.Portal.Closed || child.Portal.Transparent {
				connect(id, target.ID)
			}
		}
	}
	activeBonds := s.bonds[:0]
	for _, bond := range s.bonds {
		if bond.LastTick < tick-1 {
			continue
		}
		var locations []EntityID
		for _, id := range append(slices.Clone(bond.Left), bond.Right...) {
			if loc, ok := s.whereLocked(id); ok {
				locations = append(locations, loc.ID)
			}
		}
		if len(locations) < 2 {
			continue
		}
		for _, loc := range locations[1:] {
			connect(locations[0], loc)
		}
		activeBonds = append(activeBonds, bond)
	}
	s.bonds = activeBonds

	var components []*Scene
	visited := map[EntityID]bool{}
	for _, id := range spaces {
		if visited[id] {
			continue
		}
		visited[id] = true
		queue := []EntityID{id}
		component := &Scene{LastTick: tick, memberSince: map[EntityID]int{}}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			component.SpaceIDs = append(component.SpaceIDs, current)
			for _, next := range adjacency[current] {
				if !visited[next] {
					visited[next] = true
					queue = append(queue, next)
				}
			}
		}
		slices.Sort(component.SpaceIDs)
		for actorID, entity := range s.Entities {
			if entity.Mind == nil {
				continue
			}
			if loc, ok := s.whereLocked(actorID); ok && slices.Contains(component.SpaceIDs, loc.ID) {
				component.ActorIDs = append(component.ActorIDs, actorID)
			}
		}
		if len(component.ActorIDs) == 0 {
			continue
		} // Empty spaces belong to topology, not persistent scenes.
		slices.Sort(component.ActorIDs)
		components = append(components, component)
	}

	// Assign each previous scene to its largest surviving fragment BEFORE choosing
	// identities for merges. A small, lexically earlier room cannot steal the ID.
	owners := map[SceneID]int{}
	previous := make([]*Scene, 0, len(s.Scenes))
	for _, scene := range s.Scenes {
		previous = append(previous, scene)
	}
	sort.Slice(previous, func(i, j int) bool { return olderScene(previous[i], previous[j]) })
	for _, prev := range previous {
		best, bestCount, bestSince := -1, 0, int(^uint(0)>>1)
		for i, comp := range components {
			count, since := 0, int(^uint(0)>>1)
			for _, id := range comp.ActorIDs {
				if slices.Contains(prev.ActorIDs, id) {
					count++
					joined := prev.memberSince[id]
					if joined < since {
						since = joined
					}
					comp.memberSince[id] = joined
				}
			}
			if count > bestCount || (count > 0 && count == bestCount && since < bestSince) {
				best, bestCount, bestSince = i, count, since
			}
		}
		if best >= 0 {
			owners[prev.ID] = best
		}
	}
	next := map[SceneID]*Scene{}
	for i, comp := range components {
		for _, prev := range previous {
			if owner, ok := owners[prev.ID]; ok && owner == i {
				comp.ID, comp.CreatedTick, comp.sequence = prev.ID, prev.CreatedTick, prev.sequence
				break // Previous scenes are ordered oldest first.
			}
		}
		if comp.ID == "" {
			s.nextSceneID++
			comp.ID = SceneID(fmt.Sprintf("scene-%d", s.nextSceneID))
			comp.CreatedTick, comp.sequence = tick, s.nextSceneID
		}
		for _, id := range comp.ActorIDs {
			if _, ok := comp.memberSince[id]; !ok {
				comp.memberSince[id] = tick
			}
		}
		next[comp.ID] = comp
	}
	s.Scenes = next
	return next
}

func (s *Simulation) SceneForActor(id EntityID) (*Scene, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sceneForActorLocked(id)
}
func (s *Simulation) sceneForActorLocked(id EntityID) (*Scene, bool) {
	for _, scene := range s.Scenes {
		if slices.Contains(scene.ActorIDs, id) {
			return scene, true
		}
	}
	return nil, false
}

func (s *Simulation) bondScenesLocked(entity EntityID, older, newer SceneID) {
	left, right := s.Scenes[older], s.Scenes[newer]
	if left == nil || right == nil {
		return
	}
	for i := range s.bonds {
		bond := &s.bonds[i]
		if bond.Entity == entity && slices.Equal(bond.Left, left.ActorIDs) && slices.Equal(bond.Right, right.ActorIDs) {
			bond.LastTick = s.Clock.TickCount
			return
		}
	}
	s.bonds = append(s.bonds, sceneBond{Entity: entity, Left: slices.Clone(left.ActorIDs), Right: slices.Clone(right.ActorIDs), LastTick: s.Clock.TickCount})
}

// A conflict guarantees one merged tick. Continuing attention from both sides
// renews it; otherwise topology takes over on the following tick.
func (s *Simulation) renewBondsLocked(targets map[EntityID][]EntityID, tick int) {
	for i := range s.bonds {
		bond := &s.bonds[i]
		touches := func(group []EntityID) bool {
			for _, id := range group {
				if slices.Contains(targets[id], bond.Entity) {
					return true
				}
			}
			return false
		}
		if touches(bond.Left) && touches(bond.Right) {
			bond.LastTick = tick
		}
	}
}
