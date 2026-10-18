package scenario

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"omnia.adityagupta.dev/pkg/actor"
	"omnia.adityagupta.dev/pkg/llm"
	"omnia.adityagupta.dev/pkg/memory"
	"omnia.adityagupta.dev/pkg/scenario/spec"
	"omnia.adityagupta.dev/pkg/sim"
)

// BuildOptions configures the resolution and construction of a live simulation instance.
type BuildOptions struct {
	Client      llm.Client   // Injected LLM client (required)
	Model       string       // Default "gemini-3.5-flash-lite"
	Start       string       // "" selects the first start defined in the scenario
	Player      string       // Overrides start.PlayerID; "" uses authored default
	Interactive bool         // Attaches a HumanController to the player character
	Seed        *int64       // Overrides start.Seed if provided
	SimOptions  []sim.Option // Additional simulation options (e.g. tracing clients)
}

// Instance packages the fully initialized, running simulation world and minds.
type Instance struct {
	World     *sim.Simulation
	HumanCtrl *sim.HumanController
	Player    sim.EntityID
	Actors    map[sim.EntityID]*actor.Actor
	Start     *spec.Start
	Doc       *spec.Document
}

// Build resolves a scenario specification into an active Simulation instance and actor minds.
func Build(doc *spec.Document, opts BuildOptions) (*Instance, error) {
	if doc == nil {
		return nil, fmt.Errorf("build: document cannot be nil")
	}
	if len(doc.Starts) == 0 {
		return nil, fmt.Errorf("build: scenario defines no start points")
	}

	model := opts.Model
	if model == "" {
		model = "gemini-3.5-flash-lite"
	}

	// 1. Select Start Point
	var chosenStart *spec.Start
	if opts.Start == "" {
		chosenStart = &doc.Starts[0]
	} else {
		for i := range doc.Starts {
			if doc.Starts[i].ID == opts.Start {
				chosenStart = &doc.Starts[i]
				break
			}
		}
		if chosenStart == nil {
			return nil, fmt.Errorf("build: start point %q not found in scenario", opts.Start)
		}
	}

	startTime, err := time.Parse(time.RFC3339, chosenStart.Clock.StartAt)
	if err != nil {
		return nil, fmt.Errorf("build: invalid start time RFC 3339 %q: %w", chosenStart.Clock.StartAt, err)
	}

	clock := sim.Clock{
		Cycle:        chosenStart.Clock.Cycle,
		StartTime:    startTime,
		TickDuration: time.Duration(chosenStart.Clock.TickSeconds) * time.Second,
		TickCount:    1,
	}

	seed := int64(42)
	if chosenStart.Seed != nil {
		seed = *chosenStart.Seed
	}
	if opts.Seed != nil {
		seed = *opts.Seed
	}

	// 2. Initialize Simulation World
	var simOpts []sim.Option
	simOpts = append(simOpts, sim.WithSeed(seed))
	if opts.Client != nil {
		simOpts = append(simOpts,
			sim.WithSummarizer(sim.NewLLMSummarizer(opts.Client, model)),
			sim.WithArbiter(sim.NewLLMArbiter(opts.Client, model)),
			sim.WithBroadcaster(sim.NewLLMBroadcaster(opts.Client, model)),
			sim.WithDistiller(sim.NewLLMDistiller(opts.Client, model)),
			sim.WithConsolidator(sim.NewLLMConsolidator(opts.Client, model)),
		)
	}
	simOpts = append(simOpts, opts.SimOptions...)

	world := sim.New(clock, simOpts...)

	// 3. Resolve Present Entities and Overrides
	presentSet := make(map[string]bool)
	if len(chosenStart.PresentIDs) > 0 {
		for _, pid := range chosenStart.PresentIDs {
			presentSet[pid] = true
		}
	} else {
		for _, e := range doc.Entities {
			presentSet[e.ID] = true
		}
	}

	overrideMap := make(map[string]spec.EntityOverride)
	for _, ov := range chosenStart.Overrides {
		overrideMap[ov.EntityID] = ov
	}

	// Prepare Character Lookup
	charMap := make(map[string]spec.Character)
	for _, c := range doc.Characters {
		charMap[c.ID] = c
	}

	// Prepare Lifetime Mind Lookup
	mindMap := make(map[string]spec.Mind)
	for _, m := range doc.Minds {
		mindMap[m.OwnerID] = m
	}

	// Prepare Mind Overrides Lookup
	mindOverrideMap := make(map[string]spec.MindOverride)
	for _, mo := range chosenStart.Minds {
		mindOverrideMap[mo.OwnerID] = mo
	}

	actorsMap := make(map[sim.EntityID]*actor.Actor)

	// Helper to resolve entity fields with overrides applied
	resolveEntity := func(e spec.Entity) spec.Entity {
		if ov, ok := overrideMap[e.ID]; ok {
			if ov.ParentID != "" {
				e.ParentID = ov.ParentID
			}
			if ov.Link != nil {
				e.Link = ov.Link
			}
			if ov.Salience != nil {
				e.Salience = *ov.Salience
			}
			if ov.Space != nil {
				e.Space = ov.Space
			}
			if ov.Portal != nil {
				e.Portal = ov.Portal
			}
			if len(ov.SetAttributes) > 0 || len(ov.RemoveAttributes) > 0 {
				attrMap := make(map[string]spec.Attribute)
				for _, a := range e.Attributes {
					attrMap[a.Key] = a
				}
				for _, rem := range ov.RemoveAttributes {
					delete(attrMap, rem)
				}
				for _, set := range ov.SetAttributes {
					attrMap[set.Key] = set
				}
				var mergedAttrs []spec.Attribute
				for _, a := range attrMap {
					mergedAttrs = append(mergedAttrs, a)
				}
				e.Attributes = mergedAttrs
			}
		}
		return e
	}

	// 4. Spawn Spaces
	for _, e := range doc.Entities {
		if !presentSet[e.ID] || e.Kind != "space" {
			continue
		}
		resE := resolveEntity(e)
		enterable := true
		open := true
		transparent := false
		soundproof := false
		if resE.Space != nil {
			enterable = resE.Space.Enterable
			open = resE.Space.Open
			transparent = resE.Space.Transparent
			soundproof = resE.Space.Soundproof
		}

		sp := world.SpawnSpace(sim.EntityID(resE.ID), resE.Name, resE.Description, enterable)
		if sp.Space != nil {
			sp.Space.Open = open
			sp.Space.Transparent = transparent
			sp.Space.Soundproof = soundproof
		}
		if resE.Salience > 0 {
			sp.Salience = resE.Salience
		}

		// Apply Attributes
		for _, attr := range resE.Attributes {
			vis := sim.VisibilityPublic
			if attr.Visibility == "private" {
				vis = sim.VisibilityPrivate
			}
			var acc []sim.EntityID
			for _, ac := range attr.Accessors {
				acc = append(acc, sim.EntityID(ac))
			}
			_ = sp.Attributes.SetWithVisibility(attr.Key, attr.Value, vis, acc)
			if attr.Core {
				if aObj, ok := sp.Attributes[attr.Key]; ok {
					aObj.Core = true
					sp.Attributes[attr.Key] = aObj
				}
			}
		}
	}

	// 5. Spawn Non-Actor Items and Portals
	// Portals require both endpoints to be registered spaces
	var portalsToSpawn []spec.Entity
	var itemsToSpawn []spec.Entity
	var actorsToSpawn []spec.Entity

	for _, e := range doc.Entities {
		if !presentSet[e.ID] {
			continue
		}
		switch e.Kind {
		case "portal":
			portalsToSpawn = append(portalsToSpawn, e)
		case "item":
			itemsToSpawn = append(itemsToSpawn, e)
		case "actor":
			actorsToSpawn = append(actorsToSpawn, e)
		}
	}

	// Spawn Portals
	for _, e := range portalsToSpawn {
		resE := resolveEntity(e)
		if resE.Portal == nil {
			continue
		}
		fromID := sim.EntityID(resE.Portal.FromID)
		toID := sim.EntityID(resE.Portal.ToID)
		p, err := world.SpawnPortalWithOptions(
			sim.EntityID(resE.ID),
			resE.Name,
			resE.Description,
			fromID,
			toID,
			resE.Portal.Direction,
			resE.Portal.Closed,
			resE.Portal.Locked,
			resE.Portal.Transparent,
		)
		if err != nil {
			return nil, fmt.Errorf("build: failed spawning portal %q: %w", resE.ID, err)
		}
		if resE.Salience > 0 {
			p.Salience = resE.Salience
		}
		for _, attr := range resE.Attributes {
			vis := sim.VisibilityPublic
			if attr.Visibility == "private" {
				vis = sim.VisibilityPrivate
			}
			var acc []sim.EntityID
			for _, ac := range attr.Accessors {
				acc = append(acc, sim.EntityID(ac))
			}
			_ = p.Attributes.SetWithVisibility(attr.Key, attr.Value, vis, acc)
		}
	}

	// 6. Spawn Actors and Seed 3-Tier Memory Minds
	for _, e := range actorsToSpawn {
		resE := resolveEntity(e)
		charID := ""
		appearance := ""
		if resE.Actor != nil {
			charID = resE.Actor.CharacterID
			appearance = resE.Actor.Appearance
		}
		char, ok := charMap[charID]
		if !ok {
			return nil, fmt.Errorf("build: character %q not found for actor %q", charID, resE.ID)
		}

		// Assemble Actor Profile
		personality := char.Personality
		if len(char.Traits) > 0 {
			var traitDescs []string
			for _, tr := range char.Traits {
				traitDescs = append(traitDescs, fmt.Sprintf("%s: %s", tr.Name, tr.Description))
			}
			personality += " (Traits: " + strings.Join(traitDescs, "; ") + ")"
		}

		background := char.Background
		if len(char.Likes) > 0 || len(char.Dislikes) > 0 {
			var prefParts []string
			for _, l := range char.Likes {
				prefParts = append(prefParts, fmt.Sprintf("Likes %s (%s)", l.Item, l.Reason))
			}
			for _, d := range char.Dislikes {
				prefParts = append(prefParts, fmt.Sprintf("Dislikes %s (%s)", d.Item, d.Reason))
			}
			if background != "" {
				background += " "
			}
			background += strings.Join(prefParts, ". ") + "."
		}

		actorProfile := actor.Profile{
			ID:          char.ID,
			Name:        char.Name,
			Personality: personality,
			Directives:  char.Directives,
			Background:  background,
		}

		// Build Mind
		actorMind := memory.NewMind(char.ID)

		// A. Seed Lifetime Subjects
		lifetimeMind, hasLifetimeMind := mindMap[char.ID]
		if hasLifetimeMind {
			for _, s := range lifetimeMind.Subjects {
				var bindings []memory.Binding
				for _, b := range s.Bindings {
					bindings = append(bindings, memory.Binding{
						EntityID:   b.EntityID,
						Mode:       memory.BindingMode(b.Mode),
						Confidence: b.Confidence,
					})
				}
				actorMind.AddSubject(&memory.Subject{
					ID:       s.ID,
					Kind:     memory.SubjectKind(s.Kind),
					Names:    s.Names,
					Bindings: bindings,
					Origin:   memory.SubjectOrigin(s.Origin),
				})
			}
		}

		// Apply Start Subject Overrides
		startMindOv, hasStartMindOv := mindOverrideMap[char.ID]
		if hasStartMindOv {
			for _, s := range startMindOv.UpsertSubjects {
				var bindings []memory.Binding
				for _, b := range s.Bindings {
					bindings = append(bindings, memory.Binding{
						EntityID:   b.EntityID,
						Mode:       memory.BindingMode(b.Mode),
						Confidence: b.Confidence,
					})
				}
				actorMind.AddSubject(&memory.Subject{
					ID:       s.ID,
					Kind:     memory.SubjectKind(s.Kind),
					Names:    s.Names,
					Bindings: bindings,
					Origin:   memory.SubjectOrigin(s.Origin),
				})
			}
		}

		// B. Seed Lifetime Episodes (Sorted chronologically, filtered by end time & exclusions)
		if hasLifetimeMind {
			excludedEps := make(map[string]bool)
			if hasStartMindOv {
				for _, exID := range startMindOv.EpisodeExcludeIDs {
					excludedEps[exID] = true
				}
			}

			var eligibleEps []spec.Episode
			for _, ep := range lifetimeMind.Episodes {
				if excludedEps[ep.ID] {
					continue
				}
				epEnd, err := time.Parse(time.RFC3339, ep.End)
				if err != nil {
					continue
				}
				if epEnd.After(startTime) {
					continue // Future memory from perspective of this start point
				}
				eligibleEps = append(eligibleEps, ep)
			}

			// Sort by start time ascending
			sort.Slice(eligibleEps, func(i, j int) bool {
				return eligibleEps[i].Start < eligibleEps[j].Start
			})

			for _, ep := range eligibleEps {
				st, _ := time.Parse(time.RFC3339, ep.Start)
				et, _ := time.Parse(time.RFC3339, ep.End)
				var beats []memory.Beat
				for _, b := range ep.Beats {
					beats = append(beats, memory.Beat{
						Text:       b.Text,
						SubjectIDs: b.SubjectIDs,
					})
				}
				actorMind.AddEpisode(&memory.Episode{
					ID:             ep.ID,
					OwnerID:        char.ID,
					Source:         memory.SourceAuthored,
					Status:         memory.EpisodeClosed,
					Start:          st,
					End:            et,
					Precision:      memory.Precision(ep.Precision),
					Label:          ep.Label,
					PlaceIDs:       ep.PlaceIDs,
					ParticipantIDs: ep.ParticipantIDs,
					Title:          ep.Title,
					Summary:        ep.Summary,
					Tone:           ep.Tone,
					Importance:     ep.Importance,
					Beats:          beats,
				})
			}
		}

		// C. Seed Lifetime Dossiers & Claims
		if hasLifetimeMind {
			for _, d := range lifetimeMind.Dossiers {
				for _, c := range d.Claims {
					actorMind.AddClaim(d.SubjectID, memory.Claim{
						ID:         c.ID,
						Kind:       memory.ClaimKind(c.Kind),
						Content:    c.Content,
						Salience:   c.Salience,
						Conviction: c.Conviction,
						Fixed:      c.Fixed,
						Evidence:   c.Evidence,
						Formed:     1,
					})
				}
			}
		}

		// D. Apply Start Claims Overlays & Retractions
		if hasStartMindOv {
			for _, up := range startMindOv.UpsertClaims {
				actorMind.AddClaim(up.SubjectID, memory.Claim{
					ID:         up.Claim.ID,
					Kind:       memory.ClaimKind(up.Claim.Kind),
					Content:    up.Claim.Content,
					Salience:   up.Claim.Salience,
					Conviction: up.Claim.Conviction,
					Fixed:      up.Claim.Fixed,
					Evidence:   up.Claim.Evidence,
					Formed:     1,
				})
			}
			for _, retID := range startMindOv.RetractClaimIDs {
				actorMind.RetractClaim(retID)
			}
		}

		// Initialize Actor Entity in World
		act := actor.New(actorProfile, actor.WithMind(actorMind))
		salience := 0.8
		if resE.Salience > 0 {
			salience = resE.Salience
		}

		actEnt, err := world.SpawnActorWithOptions(
			sim.EntityID(resE.ID),
			act,
			sim.EntityID(resE.ParentID),
			appearance,
			salience,
		)
		if err != nil {
			return nil, fmt.Errorf("build: failed spawning actor %q: %w", resE.ID, err)
		}

		// Set Actor Attributes
		for _, attr := range resE.Attributes {
			vis := sim.VisibilityPublic
			if attr.Visibility == "private" {
				vis = sim.VisibilityPrivate
			}
			var acc []sim.EntityID
			for _, ac := range attr.Accessors {
				acc = append(acc, sim.EntityID(ac))
			}
			_ = actEnt.Attributes.SetWithVisibility(attr.Key, attr.Value, vis, acc)
		}

		actorsMap[sim.EntityID(resE.ID)] = act
	}

	// 7. Spawn Items (Topologically order or multi-pass parent resolution)
	// Simple pass: insert with parent if parent exists, else defer
	remainingItems := itemsToSpawn
	for len(remainingItems) > 0 {
		var nextRound []spec.Entity
		progress := false
		for _, e := range remainingItems {
			resE := resolveEntity(e)
			parentID := sim.EntityID(resE.ParentID)
			_, parentExists := world.Entities[parentID]

			if parentID == "" || parentExists {
				rel := sim.RelIn
				placement := ""
				concealed := false
				if resE.Link != nil {
					rel = sim.Relation(resE.Link.Relation)
					placement = resE.Link.Placement
					concealed = resE.Link.Concealed
				}
				it, err := world.SpawnEntity(
					sim.EntityID(resE.ID),
					resE.Name,
					resE.Description,
					parentID,
					sim.Link{Relation: rel, Placement: placement, Concealed: concealed},
				)
				if err != nil {
					return nil, fmt.Errorf("build: failed spawning entity %q: %w", resE.ID, err)
				}
				if resE.Salience > 0 {
					it.Salience = resE.Salience
				}
				for _, attr := range resE.Attributes {
					vis := sim.VisibilityPublic
					if attr.Visibility == "private" {
						vis = sim.VisibilityPrivate
					}
					var acc []sim.EntityID
					for _, ac := range attr.Accessors {
						acc = append(acc, sim.EntityID(ac))
					}
					_ = it.Attributes.SetWithVisibility(attr.Key, attr.Value, vis, acc)
				}
				progress = true
			} else {
				nextRound = append(nextRound, e)
			}
		}
		if !progress && len(nextRound) > 0 {
			return nil, fmt.Errorf("build: unresolvable item parent hierarchy for %q", nextRound[0].ID)
		}
		remainingItems = nextRound
	}

	// 8. Attach non-root container spaces (e.g. internal pockets or compartments)
	for _, e := range doc.Entities {
		if !presentSet[e.ID] || e.Kind != "space" || e.ParentID == "" {
			continue
		}
		resE := resolveEntity(e)
		rel := sim.RelIn
		placement := ""
		concealed := false
		if resE.Link != nil {
			rel = sim.Relation(resE.Link.Relation)
			placement = resE.Link.Placement
			concealed = resE.Link.Concealed
		}
		if err := world.Move(sim.EntityID(resE.ID), sim.EntityID(resE.ParentID), sim.Link{
			Relation: rel, Placement: placement, Concealed: concealed,
		}); err != nil {
			return nil, fmt.Errorf("build: failed moving container space %q to parent %q: %w", resE.ID, resE.ParentID, err)
		}
	}

	// 7. Wire Player and Controller
	playerID := chosenStart.PlayerID
	if opts.Player != "" {
		playerID = opts.Player
	}

	var humanCtrl *sim.HumanController
	if opts.Interactive && playerID != "" {
		humanCtrl = sim.NewHumanController(nil, model)
		if err := world.SetController(sim.EntityID(playerID), humanCtrl); err != nil {
			return nil, fmt.Errorf("build: failed setting controller on player %q: %w", playerID, err)
		}
	}

	// 8. Schedule Events
	for _, ev := range chosenStart.Events {
		world.ScheduleEvent(ev.Tick, sim.EntityID(ev.LocationID), ev.Text)
	}

	return &Instance{
		World:     world,
		HumanCtrl: humanCtrl,
		Player:    sim.EntityID(playerID),
		Actors:    actorsMap,
		Start:     chosenStart,
		Doc:       doc,
	}, nil
}
