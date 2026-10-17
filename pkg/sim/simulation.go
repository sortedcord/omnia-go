package sim

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"omnia.adityagupta.dev/pkg/actor"
	"omnia.adityagupta.dev/pkg/llm"
)

// Option allows configuring a Simulation during initialization.
type Option func(*Simulation)

// WithArbiter configures the physics referee that resolves intent into reality.
func WithArbiter(a Arbiter) Option {
	return func(sim *Simulation) {
		sim.Arbiter = a
	}
}

// WithBroadcaster configures the Scene Perception Broadcaster.
func WithBroadcaster(b Broadcaster) Option {
	return func(sim *Simulation) {
		sim.Broadcaster = b
	}
}

// WithDistiller configures the Thought Distiller for updating actor minds in the background.
func WithDistiller(d ThoughtDistiller) Option {
	return func(sim *Simulation) {
		sim.Distiller = d
	}
}

// WithSeed configures a fixed random seed for reproducible dice rolls.
func WithSeed(seed int64) Option {
	return func(sim *Simulation) {
		sim.Seed = seed
		sim.RNG = rand.New(rand.NewSource(seed))
	}
}

// Simulation is the top-level container managing the objective physical reality:
// the universal entity tree, LOD perception filtering, and the event-driven tick loop.
type Simulation struct {
	mu sync.RWMutex

	Clock Clock

	// Universal Entity Tree (locations, actors, vehicles, items, body parts, pockets)
	Entities map[EntityID]*Entity

	// Dynamic Perception & LOD State
	FocusMap   map[EntityID]map[EntityID]float64 // perceiverID -> targetID -> salience boost
	Summarizer Summarizer

	// Physics & Resolution
	Arbiter       Arbiter
	Broadcaster   Broadcaster
	RNG           *rand.Rand
	Seed          int64
	pendingIntent map[EntityID]string // actorID -> unfinished intent trajectory

	// Dynamic Scenes
	Scenes            map[SceneID]*Scene
	SceneBonds        map[SceneID]SceneID // olderSceneID -> newerSceneID
	ContestedEntities map[EntityID]bool
	nextSceneID       int

	// Thought Distillation
	Distiller      ThoughtDistiller
	pendingDistill map[EntityID]chan DistillResult
	lastDistill    map[EntityID]*DistillResult

	// Event Log & Loop State
	EventLog        []Event              // Canonical objective history
	inbox           map[EntityID][]Event // entityID -> pending unperceived events
	scheduledEvents map[int][]Event      // tick -> scripted world occurrences
}

// New creates an initialized, thread-safe Simulation instance.
func New(clock Clock, opts ...Option) *Simulation {
	if clock.TickCount == 0 {
		clock.TickCount = 1
	}
	if clock.TickDuration == 0 {
		clock.TickDuration = 10 * time.Second
	}
	if clock.StartTime.IsZero() {
		clock.StartTime = time.Date(2026, 10, 16, 3, 14, 0, 0, time.UTC)
	}

	seed := time.Now().UnixNano()
	defaultLLM := llm.DefaultClient()

	sim := &Simulation{
		Clock:             clock,
		Entities:          make(map[EntityID]*Entity),
		FocusMap:          make(map[EntityID]map[EntityID]float64),
		Summarizer:        NewLLMSummarizer(defaultLLM, "gemini-3.5-flash-lite"),
		Arbiter:           NewLLMArbiter(defaultLLM, "gemini-3.5-flash-lite"),
		Broadcaster:       NewLLMBroadcaster(defaultLLM, "gemini-3.5-flash-lite"),
		Distiller:         NewLLMDistiller(defaultLLM, "gemini-3.5-flash-lite"),
		Seed:              seed,
		RNG:               rand.New(rand.NewSource(seed)),
		pendingIntent:     make(map[EntityID]string),
		Scenes:            make(map[SceneID]*Scene),
		SceneBonds:        make(map[SceneID]SceneID),
		ContestedEntities: make(map[EntityID]bool),
		pendingDistill:    make(map[EntityID]chan DistillResult),
		lastDistill:       make(map[EntityID]*DistillResult),
		EventLog:          make([]Event, 0),
		inbox:             make(map[EntityID][]Event),
		scheduledEvents:   make(map[int][]Event),
	}

	for _, opt := range opts {
		opt(sim)
	}

	return sim
}

// -----------------------------------------------------------------------------
// Entity Spawning Helpers
// -----------------------------------------------------------------------------

// SpawnSpace creates and registers an enterable or container space entity.
func (s *Simulation) SpawnSpace(id EntityID, name, description string, enterable bool) *Entity {
	e := NewSpaceEntity(id, name, description, enterable)
	_ = s.AddEntity(e)
	return e
}

// SpawnActor creates an entity housing an autonomous mind and places it inside a parent entity.
func (s *Simulation) SpawnActor(id EntityID, a *actor.Actor, initialParentID EntityID, visualDescription string) (*Entity, error) {
	e := NewActorEntity(id, a, visualDescription)
	e.Parent = initialParentID
	e.Link = Link{Relation: RelIn}
	if err := s.AddEntity(e); err != nil {
		return nil, err
	}
	return e, nil
}

// SpawnEntity creates a physical entity and attaches it to a parent.
func (s *Simulation) SpawnEntity(id EntityID, name, description string, parentID EntityID, link Link) (*Entity, error) {
	e := NewItemEntity(id, name, description)
	e.Parent = parentID
	e.Link = link
	if err := s.AddEntity(e); err != nil {
		return nil, err
	}
	return e, nil
}

// SpawnItem is a backward-compatible alias for SpawnEntity.
func (s *Simulation) SpawnItem(id EntityID, name, description string, parentID EntityID, link Link) (*Entity, error) {
	return s.SpawnEntity(id, name, description, parentID, link)
}

// SpawnPortal creates an opening or passageway between two spaces.
func (s *Simulation) SpawnPortal(id EntityID, name string, fromSpaceID, toSpaceID EntityID, direction string) (*Entity, error) {
	e := NewPortalEntity(id, name, toSpaceID, direction)
	e.Parent = fromSpaceID
	e.Link = Link{Relation: RelIn}
	if err := s.AddEntity(e); err != nil {
		return nil, err
	}
	return e, nil
}

// SetController changes who or what directs an entity's decisions (LLM, Human, or nil for passive).
func (s *Simulation) SetController(entityID EntityID, c Controller) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.Entities[entityID]
	if !ok {
		return fmt.Errorf("sim: entity %q not found", entityID)
	}
	e.Controller = c
	return nil
}

// SetCondition updates an environmental attribute on a space entity.
func (s *Simulation) SetCondition(spaceID EntityID, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.Entities[spaceID]
	if !ok || e.Space == nil {
		return fmt.Errorf("sim: space %q not found", spaceID)
	}
	if e.Space.Conditions == nil {
		e.Space.Conditions = make(map[string]string)
	}
	e.Space.Conditions[key] = value
	e.Revision++
	return nil
}

// -----------------------------------------------------------------------------
// Event Management & Broadcasting
// -----------------------------------------------------------------------------

// ScheduleEvent queues a future world or environmental event for a specific tick.
func (s *Simulation) ScheduleEvent(tick int, locationID EntityID, description string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ev := Event{
		Tick:        tick,
		LocationID:  locationID,
		Kind:        EventWorld,
		Description: description,
	}
	s.scheduledEvents[tick] = append(s.scheduledEvents[tick], ev)
}

// QueueEvent queues an environmental or world event for the current tick.
func (s *Simulation) QueueEvent(locationID EntityID, description string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ev := Event{
		Tick:        s.Clock.TickCount,
		Timestamp:   s.Clock.Timestamp(),
		LocationID:  locationID,
		Kind:        EventWorld,
		Description: description,
	}
	s.scheduledEvents[s.Clock.TickCount] = append(s.scheduledEvents[s.Clock.TickCount], ev)
}

// DeliverDirectEvent routes a sensory event directly into an actor's personal inbox.
func (s *Simulation) DeliverDirectEvent(targetID EntityID, description string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	loc, _ := s.whereLocked(targetID)
	locID := EntityID("")
	if loc != nil {
		locID = loc.ID
	}

	ev := Event{
		Tick:        s.Clock.TickCount,
		Timestamp:   s.Clock.Timestamp(),
		LocationID:  locID,
		Kind:        EventWorld,
		Description: description,
	}
	s.inbox[targetID] = append(s.inbox[targetID], ev)
}

// broadcastToSpaceLocked sends an event to all actors residing inside a given space.
// Caller MUST hold s.mu.Lock().
func (s *Simulation) broadcastToSpaceLocked(spaceID EntityID, event Event, excludeID EntityID) {
	for id, entity := range s.Entities {
		if id == excludeID || entity.Mind == nil {
			continue
		}
		loc, ok := s.whereLocked(id)
		if ok && loc != nil && loc.ID == spaceID {
			s.inbox[id] = append(s.inbox[id], event)
		}
	}
}

// -----------------------------------------------------------------------------
// The Simulation Loop (Step & Run)
// -----------------------------------------------------------------------------

// Step advances the simulation by one discrete tick:
//  1. Ingests scheduled world occurrences and dispatches them to co-present actors.
//  2. Partitions the simulation into dynamic scenes based on spatial topology and conflict bonds.
//  3. Identifies active actors with pending inbox events; renders their LOD views and active thoughts.
//  4. Deliberates concurrently across all woken actors (goroutines).
//  5. Resolves target names into perceivable entity IDs and computes deterministic spontaneity scores.
//  6. Adjudicates each active scene in parallel using its scene-local Arbiter.
//  7. Applies world mutations in scene age order (oldest first), detecting boundary conflicts and bonding scenes.
//  8. Broadcasters synthesize individualized second-person perceptions in parallel per scene.
//  9. Delivers perceptions into inboxes (waking actors for next tick) and launches background thought distillers.
//  10. Decays temporary focus boosts and advances the simulation tick counter.
func (s *Simulation) Step(ctx context.Context) ([]TickReport, error) {
	s.mu.Lock()
	currentTick := s.Clock.TickCount
	currentTimestamp := s.Clock.Timestamp()

	// 1. Process scheduled world events for this tick
	if events, exists := s.scheduledEvents[currentTick]; exists {
		for _, ev := range events {
			if ev.Timestamp == "" {
				ev.Timestamp = currentTimestamp
			}
			s.EventLog = append(s.EventLog, ev)
			s.broadcastToSpaceLocked(ev.LocationID, ev, "")
		}
		delete(s.scheduledEvents, currentTick)
	}

	// 2. Build and update Scenes from spatial topology + active conflict bonds
	scenes := s.buildScenesLocked(currentTick)

	// 3. Identify active actors with pending inbox events
	type candidateActor struct {
		entity    *Entity
		spaceID   EntityID
		sceneID   SceneID
		pending   []Event
		view      RenderedPerception
		relevant  map[string]float64
		thoughts  string
		selected  []actor.Thought
		priorDist *DistillResult
	}

	var candidates []candidateActor
	for id, entity := range s.Entities {
		if entity.Controller == nil {
			delete(s.inbox, id)
			continue
		}
		pending := s.inbox[id]
		if len(pending) > 0 {
			delete(s.inbox, id)
			loc, _ := s.whereLocked(id)
			locID := EntityID("")
			if loc != nil {
				locID = loc.ID
			}
			sc, _ := s.sceneForActorLocked(id)
			scID := SceneID("")
			if sc != nil {
				scID = sc.ID
			}
			candidates = append(candidates, candidateActor{
				entity:  entity,
				spaceID: locID,
				sceneID: scID,
				pending: pending,
			})
		}
	}
	s.mu.Unlock()

	if len(candidates) == 0 {
		s.mu.Lock()
		s.Clock.Advance(1)
		s.mu.Unlock()
		return nil, nil
	}

	// 4. Await prior distillations & prepare perception per candidate
	for i := range candidates {
		c := &candidates[i]
		s.mu.Lock()
		distillChan, hasPending := s.pendingDistill[c.entity.ID]
		delete(s.pendingDistill, c.entity.ID)
		s.mu.Unlock()

		if hasPending && distillChan != nil {
			select {
			case res := <-distillChan:
				s.mu.Lock()
				s.lastDistill[c.entity.ID] = &res
				s.mu.Unlock()
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		s.mu.RLock()
		c.priorDist = s.lastDistill[c.entity.ID]
		s.mu.RUnlock()

		var eventTexts []string
		for _, ev := range c.pending {
			eventTexts = append(eventTexts, ev.Description)
		}
		s.ScanAndBoostMentions(c.entity.ID, strings.Join(eventTexts, " "))

		view, err := s.renderPerceptionView(ctx, c.entity.ID)
		if err != nil {
			return nil, fmt.Errorf("sim: failed to render perception for %q: %w", c.entity.Name, err)
		}
		c.view = view

		relevantMap := make(map[string]float64)
		for seenID, lod := range view.Seen {
			relevantMap[string(seenID)] = float64(lod)
		}
		s.mu.RLock()
		if fmap := s.FocusMap[c.entity.ID]; fmap != nil {
			for fID, boost := range fmap {
				relevantMap[string(fID)] += boost
			}
		}
		s.mu.RUnlock()
		c.relevant = relevantMap

		if c.entity.Mind != nil {
			known := make(map[string]string)
			for _, p := range view.Perceivable {
				known[string(p.ID)] = p.Name
			}
			c.entity.Mind.LearnEntities(known)
			c.thoughts = c.entity.Mind.FormatThoughtContext(currentTick, relevantMap)
			c.selected = c.entity.Mind.SelectThoughts(currentTick, relevantMap, 8)
		}
	}

	// 5. PARALLEL DELIBERATION (Every woken actor decides concurrently)
	type decisionResult struct {
		candidate candidateActor
		reaction  *actor.Reaction
		err       error
	}

	decisions := make([]decisionResult, len(candidates))
	var wg sync.WaitGroup

	for i := range candidates {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c := candidates[idx]

			var sb strings.Builder
			sb.WriteString("=== CURRENT SURROUNDINGS (Your Perspective) ===\n")
			sb.WriteString(c.view.Text)
			sb.WriteString("\n\n=== RECENT SENSORY INGRESS ===\n")
			for j, ev := range c.pending {
				if j > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(ev.FormatForPerception())
			}
			stimulusText := sb.String()

			turn := Turn{
				ActorID:   c.entity.ID,
				ActorName: c.entity.Name,
				Tick:      currentTick,
				Timestamp: currentTimestamp,
				Stimulus:  stimulusText,
				Relevant:  c.relevant,
				Thoughts:  c.thoughts,
			}

			reaction, err := c.entity.Controller.Decide(ctx, c.entity.Mind, turn)
			decisions[idx] = decisionResult{
				candidate: c,
				reaction:  reaction,
				err:       err,
			}
		}(i)
	}

	wg.Wait()

	for _, d := range decisions {
		if d.err != nil {
			return nil, fmt.Errorf("sim: actor %q failed to decide: %w", d.candidate.entity.Name, d.err)
		}
	}

	// 6. TARGET RESOLUTION & SPONTANEITY SCORING
	type resolvedTurn struct {
		candidate   candidateActor
		reaction    *actor.Reaction
		targets     []EntityID
		spontaneity float64
		luck        float64
	}

	resolvedTurns := make([]resolvedTurn, len(decisions))
	for i, d := range decisions {
		resolvedTargets := ResolveTargets(d.reaction.Action.Targets, d.candidate.view.Perceivable)

		var latestTrigger *Event
		if len(d.candidate.pending) > 0 {
			latestTrigger = &d.candidate.pending[len(d.candidate.pending)-1]
		}
		spontaneity := s.ComputeSpontaneity(d.candidate.entity.ID, currentTick, latestTrigger)

		s.mu.Lock()
		luck := s.RNG.Float64()
		s.mu.Unlock()

		resolvedTurns[i] = resolvedTurn{
			candidate:   d.candidate,
			reaction:    d.reaction,
			targets:     resolvedTargets,
			spontaneity: spontaneity,
			luck:        luck,
		}

		s.ScanAndBoostMentions(d.candidate.entity.ID, d.reaction.Action.Action)
	}

	// Group resolved turns by SceneID
	sceneTurns := make(map[SceneID][]resolvedTurn)
	for _, rt := range resolvedTurns {
		scID := rt.candidate.sceneID
		if scID == "" {
			scID = "scene-default"
		}
		sceneTurns[scID] = append(sceneTurns[scID], rt)
	}

	// 7. PER-SCENE ARBITER ADJUDICATION (In Parallel)
	type sceneAdjudicationResult struct {
		sceneID SceneID
		scene   *Scene
		ruling  *SceneRuling
		turns   []resolvedTurn
		err     error
	}

	s.mu.RLock()
	tickDur := int(s.Clock.TickDuration.Seconds())
	if tickDur <= 0 {
		tickDur = 10
	}
	recentGlobal := s.recentGlobalEventsLocked(6)
	s.mu.RUnlock()

	var arbiterWG sync.WaitGroup
	adjResults := make([]sceneAdjudicationResult, 0, len(sceneTurns))
	var adjMu sync.Mutex

	for scID, turnsInScene := range sceneTurns {
		arbiterWG.Add(1)
		go func(sceneID SceneID, rTurns []resolvedTurn) {
			defer arbiterWG.Done()

			s.mu.RLock()
			scene := s.Scenes[sceneID]
			spaceIDs := []EntityID(nil)
			if scene != nil {
				spaceIDs = scene.SpaceIDs
			}
			worldState := s.renderObjectiveSpacesLocked(spaceIDs)
			recentScene := s.recentSceneEventsLocked(spaceIDs, 5)

			intents := make([]ActorIntent, len(rTurns))
			for idx, rt := range rTurns {
				intentText := rt.reaction.Action.Action
				if prev := s.pendingIntent[rt.candidate.entity.ID]; prev != "" {
					intentText = fmt.Sprintf("%s (continuing previous intent: %s)", intentText, prev)
				}
				attrMap := make(map[string]string)
				if rt.candidate.entity.Mind != nil {
					for k, v := range rt.candidate.entity.Mind.Attributes {
						attrMap[k] = v.Value
					}
				}
				intents[idx] = ActorIntent{
					ActorID:     rt.candidate.entity.ID,
					ActorName:   rt.candidate.entity.Name,
					ActionType:  rt.reaction.Action.Type,
					ActionText:  intentText,
					Targets:     rt.targets,
					Spontaneity: rt.spontaneity,
					Luck:        rt.luck,
					Attributes:  attrMap,
				}
			}
			s.mu.RUnlock()

			adj := SceneAdjudication{
				SceneID:         sceneID,
				Tick:            currentTick,
				Timestamp:       currentTimestamp,
				Intents:         intents,
				WorldState:      worldState,
				SceneEvents:     recentScene,
				GlobalEvents:    recentGlobal,
				TickDurationSec: tickDur,
			}

			var ruling *SceneRuling
			var err error
			if s.Arbiter != nil {
				ruling, err = s.Arbiter.ResolveScene(ctx, adj)
			} else {
				var verdicts []IntentVerdict
				for _, in := range intents {
					verdicts = append(verdicts, IntentVerdict{
						ActorID: in.ActorID,
						Verdict: "as_intended",
						Outcome: "success",
					})
				}
				ruling = &SceneRuling{
					SceneID:    sceneID,
					Verdicts:   verdicts,
					Resolution: "All actions proceeded as attempted.",
				}
			}

			adjMu.Lock()
			adjResults = append(adjResults, sceneAdjudicationResult{
				sceneID: sceneID,
				scene:   scene,
				ruling:  ruling,
				turns:   rTurns,
				err:     err,
			})
			adjMu.Unlock()
		}(scID, turnsInScene)
	}

	arbiterWG.Wait()

	for _, ar := range adjResults {
		if ar.err != nil {
			return nil, fmt.Errorf("sim: scene %q arbiter failed: %w", ar.sceneID, ar.err)
		}
	}

	// 8. APPLY EFFECTS (Oldest Scene First)
	sort.Slice(adjResults, func(i, j int) bool {
		ageI, ageJ := 0, 0
		if adjResults[i].scene != nil {
			ageI = adjResults[i].scene.CreatedTick
		}
		if adjResults[j].scene != nil {
			ageJ = adjResults[j].scene.CreatedTick
		}
		return ageI < ageJ
	})

	s.mu.Lock()
	tickMutated := make(map[EntityID]SceneID)
	for _, ar := range adjResults {
		if ar.scene != nil && ar.ruling != nil {
			s.applySceneEffectsLocked(ar.scene, ar.ruling, tickMutated)
			for _, v := range ar.ruling.Verdicts {
				s.pendingIntent[v.ActorID] = v.RemainingIntent
			}
		}
	}
	s.mu.Unlock()

	// 9. PER-SCENE BROADCASTER (In Parallel)
	broadcastOutputs := make(map[SceneID]map[EntityID]string)
	var bcastWG sync.WaitGroup
	var bcastMu sync.Mutex

	for _, ar := range adjResults {
		bcastWG.Add(1)
		go func(res sceneAdjudicationResult) {
			defer bcastWG.Done()

			s.mu.RLock()
			var actorsInfo []PerceiverInfo
			sceneSpaceSet := make(map[EntityID]bool)
			if res.scene != nil {
				for _, spID := range res.scene.SpaceIDs {
					sceneSpaceSet[spID] = true
				}
			}

			for id, entity := range s.Entities {
				if entity.Mind == nil {
					continue
				}
				loc, ok := s.whereLocked(id)
				if ok && loc != nil && sceneSpaceSet[loc.ID] {
					actionTaken := ""
					for _, rt := range res.turns {
						if rt.candidate.entity.ID == id {
							actionTaken = rt.reaction.Action.Action
							break
						}
					}
					actorsInfo = append(actorsInfo, PerceiverInfo{
						ID:          id,
						Name:        entity.Name,
						SpaceName:   loc.Name,
						ActionTaken: actionTaken,
					})
				}
			}

			var actionResults []ActorActionResult
			for _, rt := range res.turns {
				verdict := "as_intended"
				if res.ruling != nil {
					for _, v := range res.ruling.Verdicts {
						if v.ActorID == rt.candidate.entity.ID {
							verdict = v.Verdict
							break
						}
					}
				}
				actionResults = append(actionResults, ActorActionResult{
					ActorID:    rt.candidate.entity.ID,
					ActorName:  rt.candidate.entity.Name,
					ActionType: rt.reaction.Action.Type,
					ActionText: rt.reaction.Action.Action,
					Verdict:    verdict,
				})
			}
			s.mu.RUnlock()

			bInput := BroadcastInput{
				SceneID:       res.sceneID,
				Tick:          currentTick,
				Timestamp:     currentTimestamp,
				Resolution:    res.ruling.Resolution,
				ActionResults: actionResults,
				Actors:        actorsInfo,
			}

			bRes, err := s.Broadcaster.Broadcast(ctx, bInput)
			if err != nil || len(bRes.Perceptions) == 0 {
				bRes.Perceptions = make(map[EntityID]string)
				for _, a := range actorsInfo {
					bRes.Perceptions[a.ID] = res.ruling.Resolution
				}
			}

			bcastMu.Lock()
			broadcastOutputs[res.sceneID] = bRes.Perceptions
			bcastMu.Unlock()
		}(ar)
	}

	bcastWG.Wait()

	// 10. DELIVER PERCEPTIONS & BUILD TICK REPORTS
	var reports []TickReport

	s.mu.Lock()
	for _, ar := range adjResults {
		sceneLoc := EntityID("")
		if ar.scene != nil && len(ar.scene.SpaceIDs) > 0 {
			sceneLoc = ar.scene.SpaceIDs[0]
		}

		ev := Event{
			Tick:        currentTick,
			Timestamp:   currentTimestamp,
			LocationID:  sceneLoc,
			SceneID:     ar.sceneID,
			Kind:        EventWorld,
			Description: ar.ruling.Resolution,
		}
		s.EventLog = append(s.EventLog, ev)

		perceptions := broadcastOutputs[ar.sceneID]
		for actorID, pText := range perceptions {
			if strings.TrimSpace(pText) != "" {
				s.inbox[actorID] = append(s.inbox[actorID], Event{
					Tick:        currentTick,
					Timestamp:   currentTimestamp,
					LocationID:  sceneLoc,
					SceneID:     ar.sceneID,
					Kind:        EventOutcome,
					Description: pText,
				})
			}
		}

		for _, rt := range ar.turns {
			reports = append(reports, TickReport{
				Tick:         currentTick,
				Timestamp:    currentTimestamp,
				SceneID:      ar.sceneID,
				ActorID:      rt.candidate.entity.ID,
				ActorName:    rt.candidate.entity.Name,
				Stimulus:     rt.candidate.view.Text,
				Reaction:     rt.reaction,
				SceneRuling:  ar.ruling,
				Thoughts:     rt.candidate.selected,
				PriorDistill: rt.candidate.priorDist,
			})
		}
	}
	s.mu.Unlock()

	// 11. LAUNCH BACKGROUND THOUGHT DISTILLERS CONCURRENTLY
	if s.Distiller != nil {
		for _, rt := range resolvedTurns {
			if rt.candidate.entity.Mind == nil {
				continue
			}
			_, isHuman := rt.candidate.entity.Controller.(*HumanController)
			distInput := DistillInput{
				ActorID:       rt.candidate.entity.ID,
				ActorName:     rt.candidate.entity.Name,
				Personality:   rt.candidate.entity.Mind.Profile.Personality,
				Tick:          currentTick,
				Timestamp:     currentTimestamp,
				Stimulus:      rt.candidate.view.Text,
				Cognition:     rt.reaction.Cognition,
				Intent:        rt.reaction.Action.Action,
				IsHuman:       isHuman,
				Thoughts:      rt.candidate.entity.Mind.ThoughtsSnapshot(),
				Perceivable:   rt.candidate.view.Perceivable,
				KnownEntities: rt.candidate.entity.Mind.KnownEntitiesCopy(),
			}

			distChan := make(chan DistillResult, 1)
			s.mu.Lock()
			s.pendingDistill[rt.candidate.entity.ID] = distChan
			s.mu.Unlock()

			go func(mind *actor.Actor, in DistillInput, out chan<- DistillResult) {
				res, err := s.Distiller.Distill(ctx, in)
				if err == nil {
					applied, rejected := mind.ApplyThoughtOps(in.Tick, res.Ops, in.KnownEntities)
					res.Applied = applied
					res.Rejected = rejected
				}
				out <- res
			}(rt.candidate.entity.Mind, distInput, distChan)
		}
	}

	_ = scenes

	// 12. ADVANCE CLOCK & DECAY FOCUS
	s.DecayFocus(0.70)

	s.mu.Lock()
	s.Clock.Advance(1)
	s.mu.Unlock()

	return reports, nil
}

// WaitDistills blocks until all in-flight background thought distillations have finished.
func (s *Simulation) WaitDistills() {
	s.mu.Lock()
	chans := make([]chan DistillResult, 0, len(s.pendingDistill))
	for id, ch := range s.pendingDistill {
		chans = append(chans, ch)
		delete(s.pendingDistill, id)
	}
	s.mu.Unlock()

	for _, ch := range chans {
		if ch != nil {
			res := <-ch
			s.mu.Lock()
			s.lastDistill[res.ActorID] = &res
			s.mu.Unlock()
		}
	}
}

// Run executes the simulation loop for up to maxTicks, or until the scene reaches steady state.
func (s *Simulation) Run(ctx context.Context, maxTicks int, onTick func(report TickReport)) ([]TickReport, error) {
	defer s.WaitDistills()
	var allReports []TickReport

	for i := 0; i < maxTicks; i++ {
		s.mu.RLock()
		hasScheduled := len(s.scheduledEvents) > 0
		hasPendingInbox := false
		for id, entity := range s.Entities {
			if entity.Controller != nil && len(s.inbox[id]) > 0 {
				hasPendingInbox = true
				break
			}
		}
		s.mu.RUnlock()

		if !hasScheduled && !hasPendingInbox {
			break
		}

		reports, err := s.Step(ctx)
		if err != nil {
			return allReports, err
		}

		for _, r := range reports {
			if onTick != nil {
				onTick(r)
			}
			allReports = append(allReports, r)
		}
	}

	return allReports, nil
}

// AdvanceClock increments tick count by ticks.
func (s *Simulation) AdvanceClock(ticks int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Clock.Advance(ticks)
}
