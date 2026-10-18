package memory

import (
	"strings"
	"testing"
	"time"
)

func TestWorkingBufferTokenBudgetAndEviction(t *testing.T) {
	buf := NewWorkingBuffer(100, 2) // small 100-token budget, 2 min verbatim turns

	// Turn size: ~40 tokens each
	t1 := Turn{Perception: "A long room description with lots of interesting items on the shelves.", Action: ActionRecord{Action: "I walk forward."}}
	t2 := Turn{Perception: "Another long room description with lots of items and sounds.", Action: ActionRecord{Action: "I stop and listen."}}
	t3 := Turn{Perception: "A third perception with some details about the doorway.", Action: ActionRecord{Action: "I look at door."}}
	t4 := Turn{Perception: "A fourth perception with sounds from the kitchen.", Action: ActionRecord{Action: "I call out."}}

	s1 := buf.Append(t1)
	s2 := buf.Append(t2)
	s3 := buf.Append(t3)
	s4 := buf.Append(t4)

	if s1 != 1 || s2 != 2 || s3 != 3 || s4 != 4 {
		t.Fatalf("expected monotonic seq 1..4, got %d, %d, %d, %d", s1, s2, s3, s4)
	}

	if buf.Len() != 4 {
		t.Fatalf("expected 4 turns, got %d", buf.Len())
	}

	// Replay should guarantee at least minVerbatimTurns (2), but trim older turns if over budget
	replay := buf.ReplayTurns()
	if len(replay) < 2 {
		t.Fatalf("expected at least 2 replay turns, got %d", len(replay))
	}

	// Eviction only happens after consolidation commits
	buf.CommitConsolidation(2)
	// After committing turn 2, turns 1 and 2 are eligible for eviction if beyond minVerbatimTurns (last 2)
	if buf.Len() > 3 {
		t.Fatalf("expected consolidated turns to be evicted, len: %d", buf.Len())
	}

	// Test degraded eviction fallback
	for i := 0; i < 10; i++ {
		buf.Append(Turn{Perception: "Flooding buffer with perceptions.", Action: ActionRecord{Action: "Waiting."}})
	}
	evicted, beatText, fromSeq, toSeq := buf.DegradedEvictionFallback()
	if len(evicted) == 0 || beatText == "" || fromSeq > toSeq {
		t.Fatalf("degraded eviction fallback failed: %d evicted, text: %q", len(evicted), beatText)
	}
}

func TestRecognitionAndExpectationMiss(t *testing.T) {
	now := time.Date(2026, 10, 16, 18, 35, 0, 0, time.UTC)

	subjects := map[string]*Subject{
		"s-home": {
			ID:    "s-home",
			Kind:  SubjectPlace,
			Names: []string{"home", "house on Elm"},
			Bindings: []Binding{
				{EntityID: "house-elm-14", Mode: BindingIdentity, Confidence: 0.95},
				{EntityID: "lot-elm-14", Mode: BindingAnchor, Confidence: 0.95},
			},
		},
		"s-zack": {
			ID:    "s-zack",
			Kind:  SubjectPerson,
			Names: []string{"Zack"},
			Bindings: []Binding{
				{EntityID: "zack", Mode: BindingIdentity, Confidence: 1.0},
			},
		},
	}

	// Percepts: Ruth stands in lot-elm-14, sees Zack and office-tower-2.
	// house-elm-14 is NOT perceived!
	percepts := []Percept{
		{EntityID: "zack", LOD: 3, Name: "Zack"},
		{EntityID: "office-tower-2", LOD: 3, Name: "Commercial Office Tower"},
	}
	locationChain := []string{"office-tower-2", "lot-elm-14"}

	recog := Recognize(subjects, percepts, locationChain, 1, now)

	// 1. Zack should be recognized via identity binding
	if recog.Recognized["s-zack"] != 1.0 {
		t.Errorf("expected s-zack recognized with conf 1.0, got %v", recog.Recognized["s-zack"])
	}

	// 2. s-home should be anchored because locationChain includes lot-elm-14
	if recog.Anchored["s-home"] != 0.95 {
		t.Errorf("expected s-home anchored with conf 0.95, got %v", recog.Anchored["s-home"])
	}

	// 3. Expectation miss: s-home is anchored at lot-elm-14, but house-elm-14 is not seen!
	if !recog.ExpectationMiss["s-home"] {
		t.Errorf("expected expectation miss on s-home")
	}
}

func TestDossierClaimDecayAndFixedProtection(t *testing.T) {
	d := &Dossier{
		SubjectID:  "s-ruth",
		Impression: "A thoughtful girl.",
		Claims: []Claim{
			{
				ID:       "c-feel",
				Kind:     ClaimFeeling,
				Content:  "Cheeks are burning.",
				Salience: 0.80,
			},
			{
				ID:         "c-belief",
				Kind:       ClaimBelief,
				Content:    "My childhood home is on Elm Street.",
				Salience:   0.80,
				Conviction: 0.90,
			},
			{
				ID:       "c-fixed",
				Kind:     ClaimBelief,
				Content:  "Fixed delusion.",
				Salience: 0.80,
				Fixed:    true,
			},
		},
	}

	t0 := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	// 30 minutes later (2 half-lives for feeling): feeling decays from 0.80 to 0.20
	t1 := t0.Add(30 * time.Minute)
	d.DecayClaims(t1, t0)

	var feelClaim, beliefClaim, fixedClaim *Claim
	for i := range d.Claims {
		switch d.Claims[i].ID {
		case "c-feel":
			feelClaim = &d.Claims[i]
		case "c-belief":
			beliefClaim = &d.Claims[i]
		case "c-fixed":
			fixedClaim = &d.Claims[i]
		}
	}

	if feelClaim == nil || feelClaim.Salience > 0.25 || feelClaim.Salience < 0.15 {
		t.Errorf("expected feeling to decay to ~0.20 in 30m, got: %+v", feelClaim)
	}
	if beliefClaim == nil || beliefClaim.Salience < 0.75 {
		t.Errorf("expected belief to stay high in 30m, got: %+v", beliefClaim)
	}
	if fixedClaim == nil || fixedClaim.Salience < 0.50 {
		t.Errorf("expected fixed claim salience to floor at 0.50, got: %+v", fixedClaim)
	}

	// Another 45 minutes later: feeling drops below 0.08 and is archived
	t2 := t1.Add(45 * time.Minute)
	d.DecayClaims(t2, t1)

	hasFeel := false
	for _, c := range d.Claims {
		if c.ID == "c-feel" {
			hasFeel = true
		}
	}
	if hasFeel {
		t.Errorf("expected feeling to be archived after 75m total elapsed")
	}

	// Verify Fixed claim cannot be retracted or updated
	err := d.ApplyOp(1, DossierOp{Op: "retract_claim", ClaimID: "c-fixed"}, "")
	if err == nil {
		t.Error("expected error attempting to retract fixed claim")
	}
	err = d.ApplyOp(1, DossierOp{Op: "upsert_claim", ClaimID: "c-fixed", Content: "New content"}, "")
	if err == nil {
		t.Error("expected error attempting to update fixed claim")
	}
}

func TestRecallAndSurfacing(t *testing.T) {
	now := time.Date(2026, 10, 16, 18, 35, 0, 0, time.UTC)

	mind := NewMind("ruth")

	// Register childhood home subject and dossier
	homeSubj := &Subject{
		ID:    "s-home",
		Kind:  SubjectPlace,
		Names: []string{"home", "house on Elm"},
		Bindings: []Binding{
			{EntityID: "house-elm-14", Mode: BindingIdentity, Confidence: 0.95},
			{EntityID: "lot-elm-14", Mode: BindingAnchor, Confidence: 0.95},
		},
	}
	mind.AddSubject(homeSubj)

	mind.AddClaim("s-home", Claim{
		ID:         "c-home-1",
		Kind:       ClaimBelief,
		Content:    "My childhood home still stands on Elm Street.",
		Salience:   0.90,
		Conviction: 0.90,
		Fixed:      true,
	})

	// Add formative childhood memory (importance 0.90)
	childhoodEp := &Episode{
		ID:             "ep-ruth-03",
		OwnerID:        "ruth",
		Source:         SourceAuthored,
		Status:         EpisodeClosed,
		Start:          now.Add(-10 * 365 * 24 * time.Hour), // 10 years ago
		End:            now.Add(-10 * 365 * 24 * time.Hour),
		Title:          "Reading on the porch swing",
		Summary:        "The summer I turned twelve, reading on the front porch with Mom.",
		PlaceIDs:       []string{"s-home"},
		ParticipantIDs: []string{"s-mom"},
		Importance:     0.90, // Formative memory: permanent access floor 0.30
		Beats: []Beat{
			{Text: "Mom brought out lemonade and we read on the swing until sunset."},
		},
	}
	mind.AddEpisode(childhoodEp)
	mind.AddDossierLink("s-home", childhoodEp.ID, 1.0, RoleFormative)

	// Ruth walks onto lot-elm-14. She sees an office tower, NOT house-elm-14!
	cues := RecallCues{
		Percepts: []Percept{
			{EntityID: "office-tower-2", LOD: 3, Name: "Commercial Office Tower"},
		},
		LocationChain: []string{"office-tower-2", "lot-elm-14"},
		CurrentSpace:  "lot-elm-14",
		CurrentTick:   1,
		CurrentTime:   now,
	}

	result := mind.Recall(cues)

	// 1. Expectation miss should have fired internally
	if len(result.ExpectationMisses) == 0 || result.ExpectationMisses[0] != "s-home" {
		t.Fatalf("expected expectation miss on s-home, got: %v", result.ExpectationMisses)
	}

	// 2. Childhood memory should surface
	if len(result.SurfacedEpisodes) == 0 {
		t.Fatal("expected ep-ruth-03 to surface, but got 0 memories")
	}
	if result.SurfacedEpisodes[0].ID != "ep-ruth-03" {
		t.Fatalf("expected surfaced episode ep-ruth-03, got %s", result.SurfacedEpisodes[0].ID)
	}

	// 3. Render prompt sections within budget: verify NO explicit "(expected here, not seen)" text appears (R2!)
	selfText, dossiersText, memoriesText := mind.RenderContext(cues)

	if strings.Contains(strings.ToLower(dossiersText), "expected") || strings.Contains(strings.ToLower(dossiersText), "not seen") {
		t.Fatalf("prompt must NOT contain explicit expectation-miss markers (R2 violation):\n%s", dossiersText)
	}

	if !strings.Contains(dossiersText, "s-home") {
		t.Fatalf("expected s-home in dossiersText:\n%s", dossiersText)
	}
	if !strings.Contains(memoriesText, "Reading on the porch swing") {
		t.Fatalf("expected childhood memory in memoriesText:\n%s", memoriesText)
	}

	_ = selfText
}

func TestSnapshotSerializationRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 16, 18, 35, 0, 0, time.UTC)
	mind := NewMind("zack")

	mind.RecordTurn(Turn{
		Tick:       1,
		Time:       now,
		Perception: "Nelly is standing by the table.",
		Action:     ActionRecord{Type: "speak", Action: "Hello Nelly"},
	})

	mind.AddSubject(&Subject{
		ID:    "s-nelly",
		Kind:  SubjectPerson,
		Names: []string{"Nelly"},
	})

	mind.AddClaim("s-nelly", Claim{
		ID:       "c-nelly-1",
		Kind:     ClaimBelief,
		Content:  "Nelly is anxious.",
		Salience: 0.85,
	})

	mind.AddEpisode(&Episode{
		ID:         "ep-zack-1",
		OwnerID:    "zack",
		Status:     EpisodeClosed,
		Title:      "Meeting Nelly",
		Summary:    "Met Nelly at the apartment.",
		Importance: 0.70,
	})

	data, err := mind.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	mind2 := NewMind("zack")
	if err := mind2.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	if mind2.Buffer().Len() != 1 {
		t.Fatalf("expected 1 turn in restored mind, got %d", mind2.Buffer().Len())
	}
	if _, ok := mind2.subjects["s-nelly"]; !ok {
		t.Fatal("expected s-nelly subject restored")
	}
	if _, ok := mind2.dossiers["s-nelly"]; !ok {
		t.Fatal("expected s-nelly dossier restored")
	}
	if _, ok := mind2.episodes["ep-zack-1"]; !ok {
		t.Fatal("expected ep-zack-1 episode restored")
	}
}

func TestConsolidationWatermarkIdempotencyAndNoLoss(t *testing.T) {
	mind := NewMind("ruth", WithBudgetTokens(50))
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mind.EnsureOpenEpisode("scene-a", "lot-a", "Lot A", 1, now)

	for i := 0; i < 8; i++ {
		mind.RecordTurn(Turn{
			SceneID:    "scene-a",
			Perception: strings.Repeat("x", 100),
			Action:     ActionRecord{Action: "Waiting"},
		})
	}

	job, ok := mind.StartConsolidation()
	if !ok {
		t.Fatal("expected consolidation job to start")
	}

	// 1. Invariant I5: Empty result must NOT commit watermark or evict turns
	mind.FinishConsolidation(job, nil)
	if mind.Buffer().LastConsolidatedSeq() != 0 || mind.Buffer().Len() != 8 {
		t.Fatalf("empty result committed through seq=%d and evicted turns, len: %d",
			mind.Buffer().LastConsolidatedSeq(), mind.Buffer().Len())
	}

	// 2. Commit valid beats
	job, ok = mind.StartConsolidation()
	if !ok {
		t.Fatal("expected consolidation job to start")
	}
	beats := []Beat{{FromSeq: job.FromSeq, ToSeq: job.ToSeq, Text: "remembered"}}
	mind.FinishConsolidation(job, beats)

	// Duplicate commit with same job must be an idempotent no-op
	mind.FinishConsolidation(job, beats)
	if len(mind.episodes[job.EpisodeID].Beats) != 1 {
		t.Fatalf("duplicate commit appended beats twice: %d beats", len(mind.episodes[job.EpisodeID].Beats))
	}

	// 3. Jobs must not mix scenes across episodes
	mind.EnsureOpenEpisode("scene-b", "lot-b", "Lot B", 2, now.Add(time.Minute))
	for i := 0; i < 5; i++ {
		mind.RecordTurn(Turn{
			SceneID:    "scene-b",
			Perception: strings.Repeat("b", 100),
			Action:     ActionRecord{Action: "Talking"},
		})
	}

	job2, ok := mind.StartConsolidation()
	if !ok {
		t.Fatal("expected job2 to start")
	}
	for _, tr := range job2.Turns {
		if tr.EpisodeID != job2.EpisodeID {
			t.Fatalf("job episode %s contains turn seq %d from episode %s", job2.EpisodeID, tr.Seq, tr.EpisodeID)
		}
	}
}

func TestNewSubjectMustNotReplaceExisting(t *testing.T) {
	mind := NewMind("ruth")
	mind.AddSubject(&Subject{
		ID:    "s-a",
		Names: []string{"Alice"},
		Bindings: []Binding{
			{EntityID: "alice", Mode: BindingIdentity, Confidence: 1},
		},
	})

	_, rejected := mind.ApplyDossierOps(1, []DossierOp{
		{Op: "new_subject", SubjectID: "s-a", Name: "Bob"},
	}, time.Time{})

	if len(rejected) == 0 {
		t.Fatal("expected duplicate new_subject to be rejected")
	}

	s, ok := mind.GetSubject("s-a")
	if !ok || s.Names[0] != "Alice" || len(s.Bindings) != 1 {
		t.Fatalf("existing subject corrupted: %+v", s)
	}
}
