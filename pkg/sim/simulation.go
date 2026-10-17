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
	RNG           *rand.Rand
	Seed          int64
	pendingIntent map[EntityID]string // actorID -> unfinished intent trajectory

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
		Clock:           clock,
		Entities:        make(map[EntityID]*Entity),
		FocusMap:        make(map[EntityID]map[EntityID]float64),
		Summarizer:      NewLLMSummarizer(defaultLLM, "gemini-3.5-flash-lite"),
		Arbiter:         NewLLMArbiter(defaultLLM, "gemini-3.5-flash-lite"),
		Distiller:       NewLLMDistiller(defaultLLM, "gemini-3.5-flash-lite"),
		Seed:            seed,
		RNG:             rand.New(rand.NewSource(seed)),
		pendingIntent:   make(map[EntityID]string),
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
//  2. Collects all active actors (entities with Mind != nil and !Passive) that have pending inbox events.
//  3. For each actor:
//     a. Scans pending events to dynamically boost LOD focus on mentioned entities.
//     b. Renders the surroundings and carried items via Level of Detail (LOD).
//     c. Delivers the combined sensory input to the actor's Mind.React().
//     d. Broadcasts manifest actions (speak/act) to co-present entities.
//  4. Decays temporary focus boosts and advances the simulation tick counter.
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

	// 2. Identify active minds with pending inbox events
	type turnInfo struct {
		entity  *Entity
		spaceID EntityID
		pending []Event
	}

	var turns []turnInfo
	for id, entity := range s.Entities {
		if entity.Controller == nil {
			// Passive entity (no controller): clear any accumulated inbox
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
			turns = append(turns, turnInfo{
				entity:  entity,
				spaceID: locID,
				pending: pending,
			})
		}
	}

	// Deterministic turn order (alphabetical by Entity ID)
	sort.Slice(turns, func(i, j int) bool {
		return turns[i].entity.ID < turns[j].entity.ID
	})
	s.mu.Unlock()

	var reports []TickReport

	// 3. Execute turns sequentially
	for _, t := range turns {
		// 3a. Await pending background distillation from previous turn for this actor
		s.mu.Lock()
		distillChan, hasPendingDistill := s.pendingDistill[t.entity.ID]
		delete(s.pendingDistill, t.entity.ID)
		s.mu.Unlock()

		if hasPendingDistill && distillChan != nil {
			select {
			case res := <-distillChan:
				s.mu.Lock()
				s.lastDistill[t.entity.ID] = &res
				s.mu.Unlock()
			case <-ctx.Done():
				return reports, ctx.Err()
			}
		}

		s.mu.RLock()
		priorDistill := s.lastDistill[t.entity.ID]
		s.mu.RUnlock()

		// Scan incoming events for mentioned entities to boost focus before rendering LOD
		var eventTexts []string
		for _, ev := range t.pending {
			eventTexts = append(eventTexts, ev.Description)
		}
		fullEventText := strings.Join(eventTexts, " ")
		s.ScanAndBoostMentions(t.entity.ID, fullEventText)

		// Render surroundings and gear at the actor's current LOD focus
		view, err := s.renderPerceptionView(ctx, t.entity.ID)
		if err != nil {
			return reports, fmt.Errorf("sim: failed to render perception for %q: %w", t.entity.Name, err)
		}

		// Build relevance map and teach perceivable entities to the actor
		relevantMap := make(map[string]float64)
		for seenID, lod := range view.Seen {
			relevantMap[string(seenID)] = float64(lod)
		}
		s.mu.RLock()
		if fmap := s.FocusMap[t.entity.ID]; fmap != nil {
			for fID, boost := range fmap {
				relevantMap[string(fID)] += boost
			}
		}
		s.mu.RUnlock()

		if t.entity.Mind != nil {
			known := make(map[string]string)
			for _, p := range view.Perceivable {
				known[string(p.ID)] = p.Name
			}
			t.entity.Mind.LearnEntities(known)
		}

		// Compile thoughts context
		thoughtsText := ""
		var selectedThoughts []actor.Thought
		if t.entity.Mind != nil {
			thoughtsText = t.entity.Mind.FormatThoughtContext(currentTick, relevantMap)
			selectedThoughts = t.entity.Mind.SelectThoughts(currentTick, relevantMap, 8)
		}

		// Compile the complete subjective stimulus
		var sb strings.Builder
		sb.WriteString("=== CURRENT SURROUNDINGS (Your Perspective) ===\n")
		sb.WriteString(view.Text)
		sb.WriteString("\n\n=== RECENT SENSORY INGRESS ===\n")
		for i, ev := range t.pending {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(ev.FormatForPerception())
		}
		stimulusText := sb.String()

		turn := Turn{
			ActorID:   t.entity.ID,
			ActorName: t.entity.Name,
			Tick:      currentTick,
			Timestamp: currentTimestamp,
			Stimulus:  stimulusText,
			Relevant:  relevantMap,
			Thoughts:  thoughtsText,
		}

		reaction, err := t.entity.Controller.Decide(ctx, t.entity.Mind, turn)
		if err != nil {
			return reports, fmt.Errorf("sim: actor %q failed to decide on tick %d: %w", t.entity.Name, currentTick, err)
		}

		// Also boost focus for any entities the actor referenced in their own action description
		s.ScanAndBoostMentions(t.entity.ID, reaction.Action.Description)

		report := TickReport{
			Tick:         currentTick,
			Timestamp:    currentTimestamp,
			ActorID:      t.entity.ID,
			ActorName:    t.entity.Name,
			Stimulus:     stimulusText,
			Reaction:     reaction,
			Thoughts:     selectedThoughts,
			PriorDistill: priorDistill,
		}

		// Launch background Thought Distiller concurrently!
		if s.Distiller != nil && t.entity.Mind != nil {
			_, isHuman := t.entity.Controller.(*HumanController)
			distInput := DistillInput{
				ActorID:       t.entity.ID,
				ActorName:     t.entity.Name,
				Personality:   t.entity.Mind.Profile.Personality,
				Tick:          currentTick,
				Timestamp:     currentTimestamp,
				Stimulus:      stimulusText,
				Cognition:     reaction.Cognition,
				Intent:        reaction.Action.Description,
				IsHuman:       isHuman,
				Thoughts:      t.entity.Mind.ThoughtsSnapshot(),
				Perceivable:   view.Perceivable,
				KnownEntities: t.entity.Mind.KnownEntitiesCopy(),
			}

			distChan := make(chan DistillResult, 1)
			s.mu.Lock()
			s.pendingDistill[t.entity.ID] = distChan
			s.mu.Unlock()

			go func(mind *actor.Actor, in DistillInput, out chan<- DistillResult) {
				res, err := s.Distiller.Distill(ctx, in)
				if err == nil {
					applied, rejected := mind.ApplyThoughtOps(in.Tick, res.Ops, in.KnownEntities)
					res.Applied = applied
					res.Rejected = rejected
				}
				out <- res
			}(t.entity.Mind, distInput, distChan)
		}
		reports = append(reports, report)

		// 4. Record action, adjudicate via Arbiter if act, and route perceptions
		switch reaction.Action.Type {
		case actor.Speak:
			s.mu.Lock()
			ev := Event{
				Tick:        currentTick,
				Timestamp:   currentTimestamp,
				LocationID:  t.spaceID,
				SourceID:    t.entity.ID,
				SourceName:  t.entity.Name,
				Kind:        EventSpeak,
				Description: reaction.Action.Description,
			}
			s.EventLog = append(s.EventLog, ev)
			s.broadcastToSpaceLocked(t.spaceID, ev, t.entity.ID)
			s.mu.Unlock()

		case actor.Act:
			if s.Arbiter != nil {
				// Prepare adjudication context under read lock
				s.mu.RLock()
				vitalsCopy := make(map[string]string)
				mood := ""
				if t.entity.Mind != nil {
					mood = t.entity.Mind.Status.Mood
					for k, v := range t.entity.Mind.Status.Vitals {
						vitalsCopy[k] = v
					}
				}
				worldState := s.renderObjectiveLocked(t.spaceID)
				recentEvents := s.recentEventsLocked(t.spaceID, 5)
				luck := s.RNG.Float64()
				tickDur := int(s.Clock.TickDuration.Seconds())
				if tickDur <= 0 {
					tickDur = 10
				}

				intent := reaction.Action.Description
				if prev := s.pendingIntent[t.entity.ID]; prev != "" {
					intent = fmt.Sprintf("%s (continuing previous intent: %s)", intent, prev)
				}
				s.mu.RUnlock()

				adj := Adjudication{
					Tick:            currentTick,
					Timestamp:       currentTimestamp,
					ActorID:         t.entity.ID,
					ActorName:       t.entity.Name,
					ActorVitals:     vitalsCopy,
					ActorMood:       mood,
					Intent:          intent,
					WorldState:      worldState,
					RecentEvents:    recentEvents,
					Luck:            luck,
					TickDurationSec: tickDur,
				}

				ruling, err := s.Arbiter.Resolve(ctx, adj)
				if err != nil {
					return reports, fmt.Errorf("sim: arbiter resolution failed on tick %d for actor %q: %w", currentTick, t.entity.Name, err)
				}

				report.Ruling = ruling

				// Apply world mutations and route sensory feeds under write lock
				s.mu.Lock()
				s.applyEffectsLocked(t.entity.ID, ruling)
				s.pendingIntent[t.entity.ID] = ruling.RemainingIntent

				// Record objective act event in audit log and broadcast to onlookers
				ev := Event{
					Tick:        currentTick,
					Timestamp:   currentTimestamp,
					LocationID:  t.spaceID,
					SourceID:    t.entity.ID,
					SourceName:  t.entity.Name,
					Kind:        EventAct,
					Description: ruling.Resolution,
					Observed:    ruling.ObserverPerception,
					Outcome:     string(ruling.Outcome),
					Intent:      reaction.Action.Description,
				}
				s.EventLog = append(s.EventLog, ev)
				s.broadcastToSpaceLocked(t.spaceID, ev, t.entity.ID)

				// Deliver personal sensory feedback and remaining intent directly to the acting actor
				feedback := ruling.ActorPerception
				if feedback == "" {
					feedback = ruling.Resolution
				}
				if ruling.RemainingIntent != "" {
					feedback += fmt.Sprintf("\n(Remaining plan not yet attempted: %s)", ruling.RemainingIntent)
				}

				outcomeEv := Event{
					Tick:        currentTick,
					Timestamp:   currentTimestamp,
					LocationID:  t.spaceID,
					SourceID:    t.entity.ID,
					SourceName:  t.entity.Name,
					Kind:        EventOutcome,
					Description: fmt.Sprintf("[Outcome of your attempt: %s]\n%s", strings.ToUpper(string(ruling.Outcome)), feedback),
					Outcome:     string(ruling.Outcome),
					Intent:      reaction.Action.Description,
				}
				s.inbox[t.entity.ID] = append(s.inbox[t.entity.ID], outcomeEv)
				s.mu.Unlock()

				// Boost focus for any entities mentioned in the ruling resolution
				s.ScanAndBoostMentions(t.entity.ID, ruling.Resolution)
			} else {
				s.mu.Lock()
				ev := Event{
					Tick:        currentTick,
					Timestamp:   currentTimestamp,
					LocationID:  t.spaceID,
					SourceID:    t.entity.ID,
					SourceName:  t.entity.Name,
					Kind:        EventAct,
					Description: reaction.Action.Description,
				}
				s.EventLog = append(s.EventLog, ev)
				s.broadcastToSpaceLocked(t.spaceID, ev, t.entity.ID)
				s.mu.Unlock()
			}

		case actor.Wait:
			s.mu.Lock()
			ev := Event{
				Tick:        currentTick,
				Timestamp:   currentTimestamp,
				LocationID:  t.spaceID,
				SourceID:    t.entity.ID,
				SourceName:  t.entity.Name,
				Kind:        EventWait,
				Description: reaction.Action.Description,
			}
			s.EventLog = append(s.EventLog, ev)
			s.mu.Unlock()
		}
	}

	// 5. Decay temporary focus boosts and advance clock
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
