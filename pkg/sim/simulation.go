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
	"omnia.adityagupta.dev/pkg/memory"
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

// WithConsolidator configures the Episodic Memory Consolidator.
func WithConsolidator(c Consolidator) Option {
	return func(sim *Simulation) {
		sim.Consolidator = c
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
	Scenes      map[SceneID]*Scene
	bonds       []sceneBond // temporary merges caused by cross-scene conflicts
	nextSceneID int
	lastTargets map[EntityID][]EntityID // actorID -> actors who targeted them last tick

	// Thought Distillation & Memory Consolidation
	Distiller      ThoughtDistiller
	Consolidator   Consolidator
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
		Clock:           clock,
		Entities:        make(map[EntityID]*Entity),
		FocusMap:        make(map[EntityID]map[EntityID]float64),
		Summarizer:      NewLLMSummarizer(defaultLLM, "gemini-3.5-flash-lite"),
		Arbiter:         NewLLMArbiter(defaultLLM, "gemini-3.5-flash-lite"),
		Broadcaster:     NewLLMBroadcaster(defaultLLM, "gemini-3.5-flash-lite"),
		Distiller:       NewLLMDistiller(defaultLLM, "gemini-3.5-flash-lite"),
		Consolidator:    NewLLMConsolidator(defaultLLM, "gemini-3.5-flash-lite"),
		Seed:            seed,
		RNG:             rand.New(rand.NewSource(seed)),
		pendingIntent:   make(map[EntityID]string),
		Scenes:          make(map[SceneID]*Scene),
		lastTargets:     make(map[EntityID][]EntityID),
		pendingDistill:  make(map[EntityID]chan DistillResult),
		lastDistill:     make(map[EntityID]*DistillResult),
		EventLog:        make([]Event, 0),
		inbox:           make(map[EntityID][]Event),
		scheduledEvents: make(map[int][]Event),
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
	if e.Attributes == nil {
		e.Attributes = make(Attributes)
	}
	if err := e.Attributes.Set(key, value); err != nil {
		return err
	}
	e.Revision++
	return nil
}

// SetConditionWithVisibility sets an environmental attribute on a space entity with ACL visibility.
func (s *Simulation) SetConditionWithVisibility(spaceID EntityID, key, value string, vis Visibility, accessors []EntityID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.Entities[spaceID]
	if !ok || e.Space == nil {
		return fmt.Errorf("sim: space %q not found", spaceID)
	}
	if e.Attributes == nil {
		e.Attributes = make(Attributes)
	}
	if err := e.Attributes.SetWithVisibility(key, value, vis, accessors); err != nil {
		return err
	}
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

// GetEventLog returns a thread-safe copy of the simulation's event log.
func (s *Simulation) GetEventLog() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]Event, len(s.EventLog))
	copy(res, s.EventLog)
	return res
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
	s.buildScenesLocked(currentTick)

	// 3. Identify active actors with pending inbox events
	type candidateActor struct {
		entity          *Entity
		spaceID         EntityID
		sceneID         SceneID
		pending         []Event
		view            RenderedPerception
		relevant        map[string]float64
		thoughts        string
		selected        []actor.Thought
		priorDist       *DistillResult
		stimulus        string
		selfContext     string
		dossiersContext string
		memoriesContext string
		recall          *memory.RecallResult
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

			if c.entity.Mind.Mind != nil {
				s.mu.RLock()
				var locChain []string
				ancestors := s.ancestorsLocked(c.entity.ID)
				for _, a := range ancestors {
					if a.Space != nil {
						locChain = append(locChain, string(a.ID))
					}
				}
				fmapCopy := make(map[string]float64)
				if fmap := s.FocusMap[c.entity.ID]; fmap != nil {
					for k, v := range fmap {
						fmapCopy[string(k)] = v
					}
				}
				s.mu.RUnlock()

				var percepts []memory.Percept
				for _, p := range view.Perceivable {
					percepts = append(percepts, memory.Percept{
						EntityID: string(p.ID),
						LOD:      int(view.Seen[p.ID]),
						Name:     p.Name,
					})
				}

				locID := string(c.spaceID)
				if locID == "" && len(locChain) > 0 {
					locID = locChain[0]
				}

				c.entity.Mind.Mind.EnsureOpenEpisode(string(c.sceneID), locID, currentTick, s.Clock.CurrentTime())

				var mentionWords []string
				for _, ev := range c.pending {
					mentionWords = append(mentionWords, strings.Fields(ev.Description)...)
				}

				cues := memory.RecallCues{
					Percepts:      percepts,
					Mentions:      mentionWords,
					FocusMap:      fmapCopy,
					LocationChain: locChain,
					CurrentSpace:  locID,
					CurrentTick:   currentTick,
					CurrentTime:   s.Clock.CurrentTime(),
				}

				recallRes := c.entity.Mind.Mind.Recall(cues)
				c.recall = &recallRes
				c.selfContext, c.dossiersContext, c.memoriesContext = memory.RenderPromptSections(recallRes, c.entity.Mind.Mind.SubjectsSnapshotMap(), 300, 700, 900)
			}
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
		c := &candidates[i]
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
		c.stimulus = sb.String()
	}

	for i := range candidates {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c := candidates[idx]
			stimulusText := c.stimulus

			var targetCandidates []string
			for _, p := range c.view.Perceivable {
				if p.ID != c.entity.ID {
					targetCandidates = append(targetCandidates, p.Name)
				}
			}

			turn := Turn{
				ActorID:          c.entity.ID,
				ActorName:        c.entity.Name,
				Tick:             currentTick,
				Timestamp:        currentTimestamp,
				Stimulus:         stimulusText,
				Relevant:         c.relevant,
				Thoughts:         c.thoughts,
				Attributes:       c.entity.Attributes.AccessibleTo(c.entity.ID),
				TargetCandidates: targetCandidates,
				SelfContext:      c.selfContext,
				DossiersContext:  c.dossiersContext,
				MemoriesContext:  c.memoriesContext,
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

		// Spontaneity reacts to what the actor perceived last tick: the latest event,
		// enriched with who targeted this actor in the previous tick's resolution.
		var trigger Event
		if len(d.candidate.pending) > 0 {
			trigger = d.candidate.pending[len(d.candidate.pending)-1]
		}
		s.mu.RLock()
		if sources := s.lastTargets[d.candidate.entity.ID]; len(sources) > 0 {
			trigger.Targets = append(trigger.Targets, d.candidate.entity.ID)
			trigger.Sources = append(trigger.Sources, sources...)
		}
		s.mu.RUnlock()
		spontaneity := s.ComputeSpontaneity(d.candidate.entity.ID, currentTick, &trigger)
		luck := deterministicRoll(s.Seed, currentTick, d.candidate.entity.ID, "luck")

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
		sceneTurns[rt.candidate.sceneID] = append(sceneTurns[rt.candidate.sceneID], rt)
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
				intents[idx] = ActorIntent{
					ActorID:     rt.candidate.entity.ID,
					ActorName:   rt.candidate.entity.Name,
					ActionType:  rt.reaction.Action.Type,
					ActionText:  intentText,
					Targets:     rt.targets,
					Spontaneity: rt.spontaneity,
					Luck:        rt.luck,
					Attributes:  rt.candidate.entity.Attributes.AccessibleTo(rt.candidate.entity.ID),
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
	targetsByActor := make(map[EntityID][]EntityID)
	nextTargets := make(map[EntityID][]EntityID)
	for _, rt := range resolvedTurns {
		targetsByActor[rt.candidate.entity.ID] = rt.targets
		for _, target := range rt.targets {
			nextTargets[target] = append(nextTargets[target], rt.candidate.entity.ID)
		}
	}
	s.lastTargets = nextTargets
	s.renewBondsLocked(targetsByActor, currentTick)
	tickMutated := make(map[EntityID]SceneID)
	for _, ar := range adjResults {
		if ar.scene != nil && ar.ruling != nil {
			orderEffects(ar.ruling)
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

			// Never fall back to the raw objective resolution: it would leak ground
			// truth (hidden items, others' outcomes) into every actor's perception.
			var bRes BroadcastResult
			var err error
			if s.Broadcaster != nil {
				bRes, err = s.Broadcaster.Broadcast(ctx, bInput)
			}
			if s.Broadcaster == nil || err != nil {
				bRes.Perceptions = minimalPerceptions(actorsInfo, actionResults)
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
		// A scene can span several spaces, so tag the event with all of them.
		// LocationID is only set when the scene has exactly one space.
		var spaceIDs []EntityID
		sceneLoc := EntityID("")
		if ar.scene != nil {
			spaceIDs = ar.scene.SpaceIDs
			if len(spaceIDs) == 1 {
				sceneLoc = spaceIDs[0]
			}
		}

		var sources, targets []EntityID
		for _, rt := range ar.turns {
			sources = append(sources, rt.candidate.entity.ID)
			targets = append(targets, rt.targets...)
		}
		ev := Event{
			Tick:        currentTick,
			Timestamp:   currentTimestamp,
			LocationID:  sceneLoc,
			SceneID:     ar.sceneID,
			SpaceIDs:    spaceIDs,
			Sources:     sources,
			Targets:     targets,
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
					SpaceIDs:    spaceIDs,
					Sources:     sources,
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
				Stimulus:     rt.candidate.stimulus,
				Reaction:     rt.reaction,
				SceneRuling:  ar.ruling,
				Thoughts:     rt.candidate.selected,
				Recall:       rt.candidate.recall,
				PriorDistill: rt.candidate.priorDist,
			})
		}
	}
	s.mu.Unlock()

	// 10. RECORD TURNS IN ACTOR MINDS (WORKING BUFFER)
	for _, rt := range resolvedTurns {
		if rt.candidate.entity.Mind != nil && rt.candidate.entity.Mind.Mind != nil {
			var percepts []memory.Percept
			for _, p := range rt.candidate.view.Perceivable {
				percepts = append(percepts, memory.Percept{
					EntityID: string(p.ID),
					LOD:      int(rt.candidate.view.Seen[p.ID]),
					Name:     p.Name,
				})
			}
			rt.candidate.entity.Mind.Mind.RecordTurn(memory.Turn{
				Tick:       currentTick,
				Time:       s.Clock.CurrentTime(),
				SceneID:    string(rt.candidate.sceneID),
				LocationID: string(rt.candidate.spaceID),
				Perception: rt.candidate.stimulus,
				Perceived:  percepts,
				Cognition:  rt.reaction.Cognition,
				Action: memory.ActionRecord{
					Type:    rt.reaction.Action.Type.String(),
					Action:  rt.reaction.Action.Action,
					Targets: rt.reaction.Action.Targets,
				},
			})
		}
	}

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
				Time:          s.Clock.CurrentTime(),
				Stimulus:      rt.candidate.stimulus,
				Cognition:     rt.reaction.Cognition,
				Intent:        rt.reaction.Action.Action,
				IsHuman:       isHuman,
				Thoughts:      rt.candidate.entity.Mind.ThoughtsSnapshot(),
				Perceivable:   rt.candidate.view.Perceivable,
				KnownEntities: rt.candidate.entity.Mind.KnownEntitiesCopy(),
				Mind:          rt.candidate.entity.Mind.Mind,
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
					if mind.Mind != nil && len(res.DossierOps) > 0 {
						mind.Mind.ApplyDossierOps(in.Tick, res.DossierOps, in.Time)
					}
				}
				out <- res
			}(rt.candidate.entity.Mind, distInput, distChan)

			// Check if actor's working buffer needs consolidation
			if s.Consolidator != nil && rt.candidate.entity.Mind != nil && rt.candidate.entity.Mind.Mind != nil {
				if job, shouldConsolidate := rt.candidate.entity.Mind.Mind.StartConsolidation(); shouldConsolidate {
					var availSubjIDs []string
					for _, s := range rt.candidate.entity.Mind.Mind.SubjectsSnapshot() {
						availSubjIDs = append(availSubjIDs, s.ID)
					}
					consInput := ConsolidateInput{
						ActorID:             rt.candidate.entity.ID,
						ActorName:           rt.candidate.entity.Name,
						Personality:         rt.candidate.entity.Mind.Profile.Personality,
						SceneID:             string(rt.candidate.sceneID),
						LocationID:          string(rt.candidate.spaceID),
						FromSeq:             job.FromSeq,
						ToSeq:               job.ToSeq,
						Turns:               job.Turns,
						IsSceneClose:        false,
						AvailableSubjectIDs: availSubjIDs,
					}
					go func(mind *actor.Actor, in ConsolidateInput, j memory.ConsolidateJob) {
						cRes, cErr := s.Consolidator.Consolidate(ctx, in)
						if cErr == nil {
							mind.Mind.FinishConsolidation(j, cRes.Beats)
						} else {
							mind.Mind.AbortConsolidation()
						}
					}(rt.candidate.entity.Mind, consInput, job)
				}
			}
		}
	}

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
	defer func() {
		s.WaitDistills()
		s.CloseAllEpisodes(context.Background())
	}()
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

// CloseAllEpisodes closes open episodes across all actors and finalizes them.
func (s *Simulation) CloseAllEpisodes(ctx context.Context) {
	type closeTask struct {
		mind   *actor.Actor
		epID   string
		input  ConsolidateInput
		buffer *memory.WorkingBuffer
		toSeq  int64
	}

	s.mu.Lock()
	var tasks []closeTask
	for _, entity := range s.Entities {
		if entity.Mind != nil && entity.Mind.Mind != nil {
			openEpID := entity.Mind.Mind.OpenEpisodeID()
			if openEpID != "" {
				entity.Mind.Mind.CloseAllOpenEpisodes(s.Clock.TickCount, s.Clock.CurrentTime())
				if s.Consolidator != nil {
					buffer := entity.Mind.Mind.Buffer()
					unconsolidated := buffer.UnconsolidatedTurns()
					loc, _ := s.whereLocked(entity.ID)
					locID := ""
					if loc != nil {
						locID = string(loc.ID)
					}
					var fromSeq, toSeq int64
					if len(unconsolidated) > 0 {
						fromSeq = unconsolidated[0].Seq
						toSeq = unconsolidated[len(unconsolidated)-1].Seq
					}
					var availSubjIDs []string
					for _, s := range entity.Mind.Mind.SubjectsSnapshot() {
						availSubjIDs = append(availSubjIDs, s.ID)
					}
					tasks = append(tasks, closeTask{
						mind:   entity.Mind,
						epID:   openEpID,
						buffer: buffer,
						toSeq:  toSeq,
						input: ConsolidateInput{
							ActorID:             entity.ID,
							ActorName:           entity.Name,
							Personality:         entity.Mind.Profile.Personality,
							SceneID:             "",
							LocationID:          locID,
							FromSeq:             fromSeq,
							ToSeq:               toSeq,
							Turns:               unconsolidated,
							IsSceneClose:        true,
							AvailableSubjectIDs: availSubjIDs,
						},
					})
				}
			}
		}
	}
	s.mu.Unlock()

	// Execute consolidations outside of s.mu.Lock()
	for _, t := range tasks {
		cRes, err := s.Consolidator.Consolidate(ctx, t.input)
		if err == nil {
			for _, b := range cRes.Beats {
				t.mind.Mind.AppendBeat(t.epID, b)
			}
			t.mind.Mind.FinalizeEpisodeClose(t.epID, cRes.Title, cRes.Summary, cRes.Tone, cRes.Importance, cRes.FormativeSubjectIDs)
			if t.toSeq > 0 {
				t.buffer.CommitConsolidation(t.toSeq)
			}
		}
	}
}

// AdvanceClock increments tick count by ticks.
func (s *Simulation) AdvanceClock(ticks int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Clock.Advance(ticks)
}
