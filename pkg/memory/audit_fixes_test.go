package memory

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAuditEmptyConsolidationDoesNotLivelock(t *testing.T) {
	mind := NewMind("ruth", WithBudgetTokens(50))
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mind.EnsureOpenEpisode("scene-1", "lot-1", "Lot 1", 1, now)

	// Append 8 turns exceeding budget (50)
	for i := 0; i < 8; i++ {
		mind.RecordTurn(Turn{
			SceneID:    "scene-1",
			Perception: strings.Repeat("x", 100),
			Action:     ActionRecord{Action: "Waiting"},
		})
	}

	// 1st empty result: watermark not advanced, empty counter = 1
	job1, ok := mind.StartConsolidation()
	if !ok {
		t.Fatal("expected job 1")
	}
	mind.FinishConsolidation(job1, nil)
	if mind.BufferLen() != 8 {
		t.Fatalf("expected 8 turns after 1st empty result, got %d", mind.BufferLen())
	}

	// 2nd empty result: counter reaches 2 -> triggers degraded eviction fallback!
	job2, ok := mind.StartConsolidation()
	if !ok {
		t.Fatal("expected job 2")
	}
	mind.FinishConsolidation(job2, nil)

	// Buffer must now be reduced/bounded, and degraded beat appended!
	if mind.BufferLen() >= 8 {
		t.Fatalf("expected buffer to be pruned after 2 consecutive empty results, got len %d", mind.BufferLen())
	}
	ep := mind.episodes[job1.EpisodeID]
	if len(ep.Beats) == 0 || !ep.Beats[0].Degraded {
		t.Fatalf("expected degraded beat appended to episode, got: %+v", ep.Beats)
	}
}

func TestAuditLivePointerSafetyAndNoRaces(t *testing.T) {
	mind := NewMind("ruth")
	mind.AddSubject(&Subject{
		ID:       "s-a",
		Names:    []string{"Alice"},
		Bindings: []Binding{{EntityID: "alice", Mode: BindingIdentity, Confidence: 1.0}},
	})
	mind.AddClaim("s-a", Claim{ID: "c-1", Kind: ClaimBelief, Content: "Belief 1", Salience: 0.90})

	// GetSubject and GetDossier return detached copies
	subj, _ := mind.GetSubject("s-a")
	doss, _ := mind.GetDossier("s-a")

	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(3)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Goroutine 1: Mutates subject and claims continuously
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 2000; i++ {
			s := 0.50
			mind.ApplyDossierOps(i, []DossierOp{
				{Op: "add_name", SubjectID: "s-a", Name: "Alias"},
				{Op: "upsert_claim", SubjectID: "s-a", ClaimID: "c-1", Content: "New belief", Salience: &s},
			}, now)
		}
	}()

	// Goroutine 2: Reads detached copies
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 2000; i++ {
			_ = len(subj.Names)
			_ = len(doss.Claims)
			if len(doss.Claims) > 0 {
				_ = doss.Claims[0].Content
			}
		}
	}()

	// Goroutine 3: Calls RenderContext concurrently
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 2000; i++ {
			cues := RecallCues{
				CurrentTick: i,
				CurrentTime: now,
			}
			mind.RenderContext(cues)
		}
	}()

	close(start)
	wg.Wait()
}

func TestAuditArchivedClaimIDNotReused(t *testing.T) {
	mind := NewMind("ruth")
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mind.AddClaim("ruth", Claim{Kind: ClaimFeeling, Content: "First feeling", Salience: 0.90})
	doss, _ := mind.GetDossier("ruth")
	firstID := doss.Claims[0].ID

	// Retract claim so it moves to ArchivedClaims
	mind.ApplyDossierOps(1, []DossierOp{{Op: "retract_claim", SubjectID: "ruth", ClaimID: firstID}}, now)

	// Reset sequence counter to test allocation collision resistance
	mind.mu.Lock()
	mind.nextClaimSeq = 1
	mind.mu.Unlock()

	mind.AddClaim("ruth", Claim{Kind: ClaimFeeling, Content: "Second feeling", Salience: 0.90})
	doss2, _ := mind.GetDossier("ruth")

	secondID := doss2.Claims[0].ID
	if firstID == secondID {
		t.Fatalf("expected unique claim IDs, but archived ID %q was reused", firstID)
	}
}

func TestAuditFallbackScopedToEpisode(t *testing.T) {
	mind := NewMind("ruth", WithBudgetTokens(50))
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Episode 1 turns
	mind.EnsureOpenEpisode("scene-1", "lot-1", "Lot 1", 1, now)
	for i := 0; i < 4; i++ {
		mind.RecordTurn(Turn{
			SceneID:    "scene-1",
			Perception: strings.Repeat("1", 100),
			Action:     ActionRecord{Action: "Action in ep 1"},
		})
	}

	// Episode 2 turns
	mind.EnsureOpenEpisode("scene-2", "lot-2", "Lot 2", 2, now.Add(15*time.Minute))
	for i := 0; i < 6; i++ {
		mind.RecordTurn(Turn{
			SceneID:    "scene-2",
			Perception: strings.Repeat("2", 100),
			Action:     ActionRecord{Action: "Action in ep 2"},
		})
	}

	// Trigger degraded fallback for ep-ruth-1
	evicted, beatText, fromSeq, toSeq := mind.buffer.DegradedEvictionFallbackForEpisode("ep-ruth-1")
	if len(evicted) == 0 || beatText == "" || fromSeq > toSeq {
		t.Fatalf("expected ep-ruth-1 turns to be evicted, got %d evicted", len(evicted))
	}

	// All evicted turns MUST belong to ep-ruth-1
	for _, tr := range evicted {
		if tr.EpisodeID != "ep-ruth-1" {
			t.Fatalf("expected only ep-ruth-1 turns evicted, got turn from %s", tr.EpisodeID)
		}
	}

	// Turns remaining in the buffer for ep-ruth-2 must be completely intact
	ep2Count := 0
	for _, tr := range mind.buffer.turns {
		if tr.EpisodeID == "ep-ruth-2" {
			ep2Count++
		}
	}
	if ep2Count != 6 {
		t.Fatalf("expected all 6 turns of ep-ruth-2 preserved, got %d", ep2Count)
	}
}

func TestAuditGetDossierDoesNotMutateOnRead(t *testing.T) {
	mind := NewMind("ruth")

	beforeCount := len(mind.DossiersSnapshot())
	d, exists := mind.GetDossier("nonexistent-subject")
	if exists || d != nil {
		t.Fatalf("expected nonexistent dossier to return nil, false")
	}

	afterCount := len(mind.DossiersSnapshot())
	if beforeCount != afterCount {
		t.Fatalf("GetDossier mutated mind on read: before=%d, after=%d", beforeCount, afterCount)
	}
}

func TestAuditCrossDossierUpsertReallocatesID(t *testing.T) {
	mind := NewMind("ruth")
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mind.AddSubject(&Subject{ID: "s-a", Names: []string{"Alice"}})
	mind.AddClaim("s-a", Claim{ID: "c-shared-1", Kind: ClaimBelief, Content: "Belief about Alice", Salience: 0.90})

	// Try to upsert into self-dossier using the same claim ID
	mind.ApplyDossierOps(1, []DossierOp{
		{Op: "upsert_claim", SubjectID: "ruth", ClaimID: "c-shared-1", Content: "Claim about Ruth"},
	}, now)

	// Verify that claim ID was reallocated to avoid duplicate IDs across dossiers
	selfDoss, _ := mind.GetDossier("ruth")
	if len(selfDoss.Claims) != 1 {
		t.Fatalf("expected 1 claim in self-dossier, got %d", len(selfDoss.Claims))
	}
	if selfDoss.Claims[0].ID == "c-shared-1" {
		t.Fatalf("expected claim ID to be re-allocated, but got duplicate ID 'c-shared-1'")
	}
}

func TestAuditEnsureOpenEpisodeDoesNotLeakStuckIsConsolidating(t *testing.T) {
	mind := NewMind("ruth", WithBudgetTokens(50))
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Episode 1
	mind.EnsureOpenEpisode("scene-1", "lot-1", "Lot 1", 1, now)
	mind.RecordTurn(Turn{SceneID: "scene-1", Perception: "turn in scene 1"})

	// Transition to Episode 2 -> closes Episode 1 and issues closedJob
	_, _, closedJob := mind.EnsureOpenEpisode("scene-2", "lot-2", "Lot 2", 2, now.Add(15*time.Minute))
	if closedJob == nil {
		t.Fatal("expected closedJob for scene-1")
	}

	// Commit consolidation of closedJob
	mind.FinishConsolidation(*closedJob, []Beat{{Text: "Beat of scene 1"}})

	// Record 10 turns in scene-2 to trigger rolling consolidation
	for i := 0; i < 10; i++ {
		mind.RecordTurn(Turn{SceneID: "scene-2", Perception: strings.Repeat("2", 100)})
	}

	// Rolling consolidation MUST be allowed (isConsolidating was cleared!)
	rollJob, ok := mind.StartConsolidation()
	if !ok {
		t.Fatal("StartConsolidation was blocked by stuck isConsolidating flag")
	}
	if rollJob.EpisodeID != "ep-ruth-2" {
		t.Fatalf("expected rolling job for ep-ruth-2, got %s", rollJob.EpisodeID)
	}
}
