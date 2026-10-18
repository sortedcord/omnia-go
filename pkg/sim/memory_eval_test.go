package sim

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"omnia.adityagupta.dev/pkg/actor"
	"omnia.adityagupta.dev/pkg/memory"
)

func TestMemoryDesyncProbe(t *testing.T) {
	// Recreate the exact Demolished Childhood Home setup from docs/plans/memory-system.md §6
	now := time.Date(2026, 10, 16, 18, 35, 0, 0, time.UTC)
	world := offlineWorld()
	world.Clock = Clock{
		Cycle:        1,
		StartTime:    now,
		TickDuration: 20 * time.Second,
		TickCount:    1,
	}

	// Physical World: lot-elm-14 contains office-tower-2. house-elm-14 is GONE.
	lot := world.SpawnSpace("lot-elm-14", "14 Elm Street (Lot)", "An open urban lot.", true)
	tower, _ := world.SpawnEntity("office-tower-2", "Glass Office Tower", "A modern commercial skyscraper.", lot.ID, Link{Relation: RelIn})

	// Ruth's Mind
	ruthProfile := actor.Profile{ID: "ruth", Name: "Ruth"}
	ruthMind := memory.NewMind("ruth")

	// Authored Subject: s-home (childhood home)
	// Identity: house-elm-14 (demolished!) | Anchor: lot-elm-14
	homeSubj := &memory.Subject{
		ID:    "s-home",
		Kind:  memory.SubjectPlace,
		Names: []string{"home", "the house on Elm", "childhood home"},
		Bindings: []memory.Binding{
			{EntityID: "house-elm-14", Mode: memory.BindingIdentity, Confidence: 0.95},
			{EntityID: "lot-elm-14", Mode: memory.BindingAnchor, Confidence: 0.95},
		},
		Origin: memory.OriginAuthored,
	}
	ruthMind.AddSubject(homeSubj)

	// Authored fixed belief
	ruthMind.AddClaim("s-home", memory.Claim{
		ID:         "c-ruth-home-1",
		Kind:       memory.ClaimBelief,
		Content:    "My childhood home still stands on Elm Street.",
		Salience:   0.90,
		Conviction: 0.95,
		Fixed:      true, // Delusion / core conviction
	})

	// Formative episode: ep-ruth-03 (summer on porch swing)
	ep03 := &memory.Episode{
		ID:             "ep-ruth-03",
		OwnerID:        "ruth",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          now.Add(-10 * 365 * 24 * time.Hour),
		End:            now.Add(-10 * 365 * 24 * time.Hour),
		Title:          "Reading on the porch swing",
		Summary:        "The summer I turned twelve, reading on the front porch with Mom.",
		PlaceIDs:       []string{"s-home"},
		ParticipantIDs: []string{"s-mom"},
		Importance:     0.90,
		Beats: []memory.Beat{
			{Text: "We read books on the wooden swing until the streetlights came on."},
		},
	}
	ruthMind.AddEpisode(ep03)
	ruthMind.AddDossierLink("s-home", ep03.ID, 1.00, memory.RoleFormative)

	ruthActor := actor.New(ruthProfile, actor.WithMind(ruthMind))
	ruthEntity, _ := world.SpawnActor("ruth", ruthActor, tower.ID, "A woman in a coat, looking around in bewilderment.")
	ruthEntity.Controller = fakeController{action: actor.Action{Type: actor.Wait, Action: "Looking around in shock."}}
	world.Arbiter = fakeArbiter{ruling: SceneRuling{Resolution: "Ruth looks around in shock."}}

	// Queue arrival event to wake Ruth
	world.DeliverDirectEvent(ruthEntity.ID, "You step out of the taxi onto 14 Elm Street.")

	reports, err := world.Step(context.Background())
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}

	if len(reports) == 0 {
		t.Fatal("expected at least 1 report for Ruth")
	}

	rep := reports[0]
	if rep.Recall == nil {
		t.Fatal("expected non-nil Recall in TickReport")
	}

	// 1. Assert expectation miss fired for s-home
	hasMiss := false
	for _, missID := range rep.Recall.ExpectationMisses {
		if missID == "s-home" {
			hasMiss = true
			break
		}
	}
	if !hasMiss {
		t.Errorf("expected expectation miss on s-home, got: %v", rep.Recall.ExpectationMisses)
	}

	// 2. Assert formative childhood episode ep-ruth-03 surfaced
	hasEp03 := false
	for _, ep := range rep.Recall.SurfacedEpisodes {
		if ep.ID == "ep-ruth-03" {
			hasEp03 = true
			break
		}
	}
	if !hasEp03 {
		t.Errorf("expected ep-ruth-03 to surface, got: %v", rep.Recall.SurfacedEpisodes)
	}

	// 3. Assert R2: prompt sections contain NO explicit "(expected here, not seen)" text
	cues := memory.RecallCues{
		Percepts: []memory.Percept{
			{EntityID: "office-tower-2", LOD: 3, Name: "Glass Office Tower"},
		},
		LocationChain: []string{"office-tower-2", "lot-elm-14"},
		CurrentSpace:  "lot-elm-14",
		CurrentTick:   1,
		CurrentTime:   now,
	}
	_, dossiersText, _ := ruthMind.RenderContext(cues)
	if strings.Contains(strings.ToLower(dossiersText), "expected") || strings.Contains(strings.ToLower(dossiersText), "not seen") {
		t.Errorf("dossiersText violates R2 by containing explicit marker:\n%s", dossiersText)
	}

	// 4. Assert Fixed belief survives
	dossAfter, _ := ruthMind.GetDossier("s-home")
	foundFixed := false
	for _, c := range dossAfter.Claims {
		if c.ID == "c-ruth-home-1" && c.Fixed {
			foundFixed = true
			break
		}
	}
	if !foundFixed {
		t.Errorf("expected fixed belief to survive in dossier")
	}
}

func TestMemoryRecallProbeOnMention(t *testing.T) {
	now := time.Date(2026, 10, 16, 18, 35, 0, 0, time.UTC)
	world := offlineWorld()
	world.Clock = Clock{
		Cycle:        1,
		StartTime:    now,
		TickDuration: 20 * time.Second,
		TickCount:    1,
	}

	room := world.SpawnSpace("living-room", "Living Room", "A quiet room.", true)

	nellyProfile := actor.Profile{ID: "nelly", Name: "Nelly"}
	nellyMind := memory.NewMind("nelly")

	// Subject: s-leo (Nelly's ex). He is NOT in the simulation world!
	leoSubj := &memory.Subject{
		ID:     "s-leo",
		Kind:   memory.SubjectPerson,
		Names:  []string{"Leo", "my ex", "Leonard"},
		Origin: memory.OriginAuthored,
	}
	nellyMind.AddSubject(leoSubj)

	nellyMind.AddClaim("s-leo", memory.Claim{
		ID:         "c-leo-1",
		Kind:       memory.ClaimSuspicion,
		Content:    "Leo left without telling anyone and put me in this mess.",
		Salience:   0.85,
		Conviction: 0.90,
	})

	// Authored episode about Leo
	leoEp := &memory.Episode{
		ID:             "ep-nelly-leo-breakup",
		OwnerID:        "nelly",
		Source:         memory.SourceAuthored,
		Status:         memory.EpisodeClosed,
		Start:          now.Add(-60 * 24 * time.Hour), // 2 months ago
		End:            now.Add(-60 * 24 * time.Hour),
		Title:          "Leo packs his bags",
		Summary:        "Leo packed up his car while I was at class and left his keys on the counter.",
		ParticipantIDs: []string{"s-leo"},
		Importance:     0.88,
	}
	nellyMind.AddEpisode(leoEp)
	nellyMind.AddDossierLink("s-leo", leoEp.ID, 1.0, memory.RoleParticipant)

	nellyActor := actor.New(nellyProfile, actor.WithMind(nellyMind))
	nellyEntity, _ := world.SpawnActor("nelly", nellyActor, room.ID, "A girl in a hoodie.")
	nellyEntity.Controller = fakeController{action: actor.Action{Type: actor.Wait, Action: "Listening to Mia."}}
	world.Arbiter = fakeArbiter{ruling: SceneRuling{Resolution: "Nelly listens."}}

	// Sensory ingress mentions "Leo" explicitly!
	world.DeliverDirectEvent(nellyEntity.ID, "Mia yells from the other room: 'Did Leo ever pay his half of the deposit?'")

	reports, err := world.Step(context.Background())
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}

	if len(reports) == 0 {
		t.Fatal("expected report for Nelly")
	}

	rep := reports[0]
	if rep.Recall == nil {
		t.Fatal("expected non-nil Recall")
	}

	// Verify s-leo dossier activated
	leoActivated := false
	for _, d := range rep.Recall.ActivatedDossiers {
		if d.SubjectID == "s-leo" {
			leoActivated = true
			break
		}
	}
	if !leoActivated {
		t.Errorf("expected s-leo dossier to activate on name mention")
	}

	// Verify ep-nelly-leo-breakup surfaced
	leoEpSurfaced := false
	for _, ep := range rep.Recall.SurfacedEpisodes {
		if ep.ID == "ep-nelly-leo-breakup" {
			leoEpSurfaced = true
			break
		}
	}
	if !leoEpSurfaced {
		t.Errorf("expected breakup episode to surface on name mention")
	}
}

func TestMemoryWorkingBufferBoundednessAndWatermark(t *testing.T) {
	buf := memory.NewWorkingBuffer(200, 3) // 200 token budget, 3 min verbatim turns

	// Simulate 40 turns appending
	for i := 1; i <= 40; i++ {
		buf.Append(memory.Turn{
			Tick:       i,
			Perception: fmt.Sprintf("Turn %d: A fairly detailed perception of the environment with various ongoing activities.", i),
			Action:     memory.ActionRecord{Type: "speak", Action: fmt.Sprintf("I say sentence %d.", i)},
		})

		// Every 10 turns, simulate committed consolidation
		if i%10 == 0 {
			buf.CommitConsolidation(int64(i - 3))
		}
	}

	// Buffer must stay bounded (not all 40 turns remain!)
	if buf.Len() >= 40 {
		t.Errorf("working buffer failed to evict consolidated turns: len=%d", buf.Len())
	}

	// Replay turns must respect minVerbatimTurns
	replay := buf.ReplayTurns()
	if len(replay) < 3 {
		t.Errorf("expected at least 3 verbatim replay turns, got %d", len(replay))
	}
}

func TestDestroyedEntityPreservesMindBindingsAndClaims(t *testing.T) {
	now := time.Date(2026, 10, 16, 18, 35, 0, 0, time.UTC)
	world := offlineWorld()
	world.Clock = Clock{TickCount: 1, StartTime: now, TickDuration: 20 * time.Second}

	room := world.SpawnSpace("room-1", "Room 1", "A room.", true)
	item, _ := world.SpawnEntity("antique-clock", "Antique Clock", "A grandfather clock.", room.ID, Link{Relation: RelOn})

	actorMind := memory.NewMind("zack")
	actorMind.AddSubject(&memory.Subject{
		ID:    "s-clock",
		Kind:  memory.SubjectObject,
		Names: []string{"Antique Clock", "Grandfather Clock"},
		Bindings: []memory.Binding{
			{EntityID: "antique-clock", Mode: memory.BindingIdentity, Confidence: 1.0},
		},
	})
	actorMind.AddClaim("s-clock", memory.Claim{
		ID:         "c-clock-1",
		Kind:       memory.ClaimBelief,
		Content:    "The clock chimes loudly on the hour.",
		Salience:   0.85,
		Conviction: 0.90,
	})

	snapBefore, err := actorMind.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	// Remove/destroy the entity in the simulation world
	if err := world.RemoveEntity(item.ID); err != nil {
		t.Fatalf("failed to remove entity: %v", err)
	}

	if _, exists := world.GetEntity("antique-clock"); exists {
		t.Fatal("expected antique-clock to be removed from simulation")
	}

	snapAfter, err := actorMind.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	// Invariant I2: The world never writes to memory. Mind must be 100% byte-identical!
	if string(snapBefore) != string(snapAfter) {
		t.Fatalf("Invariant I2 violation: actor memory mutated when world entity was destroyed!\nBefore: %s\nAfter:  %s",
			snapBefore, snapAfter)
	}
}

func TestRunClosesOpenEpisodes(t *testing.T) {
	world := offlineWorld()
	world.Consolidator = nil
	room := world.SpawnSpace("room", "Room", "", true)
	a := actor.New(actor.Profile{ID: "x", Name: "X"})
	ent, err := world.SpawnActor("x", a, room.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	ent.Controller = fakeController{action: actor.Action{Type: actor.Wait, Action: "wait"}}
	world.DeliverDirectEvent("x", "a sound")

	if _, err = world.Run(context.Background(), 1, nil); err != nil {
		t.Fatal(err)
	}

	eps := a.Mind.EpisodesSnapshot()
	if len(eps) != 1 || eps[0].Status != memory.EpisodeClosed {
		t.Fatalf("Run ended without closing episode: %+v", eps)
	}
}

type mockMidRunConsolidator struct{}

func (m mockMidRunConsolidator) Consolidate(_ context.Context, in ConsolidateInput) (ConsolidateResult, error) {
	return ConsolidateResult{
		Beats:      []memory.Beat{{Text: "Beat in scene"}},
		Title:      "Finalized Scene Title",
		Summary:    "First-person summary of what occurred in the scene.",
		Tone:       "thoughtful",
		Importance: 0.80,
	}, nil
}

func TestMidRunEpisodeFinalization(t *testing.T) {
	world := offlineWorld()
	world.Clock = Clock{
		TickCount:    1,
		StartTime:    time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		TickDuration: 20 * time.Second,
	}
	world.Consolidator = mockMidRunConsolidator{}

	roomA := world.SpawnSpace("room-a", "Room A", "", true)
	roomB := world.SpawnSpace("room-b", "Room B", "", true)
	door, _ := world.SpawnPortal("door-ab", "Door AB", roomA.ID, roomB.ID, "")
	door.Portal.Closed = true

	a := actor.New(actor.Profile{ID: "x", Name: "X"})
	ent, _ := world.SpawnActor("x", a, roomA.ID, "")
	ent.Controller = fakeController{action: actor.Action{Type: actor.Wait, Action: "wait"}}

	// Tick 1 in room-a (scene-1)
	world.DeliverDirectEvent("x", "sound in room a")
	world.Step(context.Background())

	// Advance clock by 15 minutes (45 ticks of 20s) so time gap exceeds 10 minutes
	world.AdvanceClock(45)
	world.DeliverDirectEvent("x", "sound in room a 15 minutes later")

	// Tick 2: In-world time gap > 10 min closes scene-1's episode and opens scene-2's episode!
	world.Step(context.Background())
	world.WaitConsolidations()

	eps := a.Mind.EpisodesSnapshot()
	if len(eps) < 2 {
		t.Fatalf("expected at least 2 episodes, got %d", len(eps))
	}

	var firstEp *memory.Episode
	for _, ep := range eps {
		if ep.PrevID == "" {
			firstEp = ep
			break
		}
	}

	if firstEp == nil || firstEp.Status != memory.EpisodeClosed {
		t.Fatalf("expected first episode to be closed: %+v", firstEp)
	}
	if firstEp.Title != "Finalized Scene Title" {
		t.Errorf("expected finalized title, got: %q", firstEp.Title)
	}
	if firstEp.Summary != "First-person summary of what occurred in the scene." {
		t.Errorf("expected finalized summary, got: %q", firstEp.Summary)
	}
}
