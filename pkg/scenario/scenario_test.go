package scenario

import (
	"os"
	"testing"
	"time"

	"omnia.adityagupta.dev/pkg/memory"
	"omnia.adityagupta.dev/pkg/sim"
)

func TestScenarioAuthoringDepth(t *testing.T) {
	// Dummy credentials for offline test initialization
	if os.Getenv("OMNIA_API_KEY") == "" && os.Getenv("OPENAI_API_KEY") == "" {
		_ = os.Setenv("OMNIA_API_KEY", "dummy-key-for-offline-test")
	}

	sc, err := Setup(false)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// 1. Verify World Topology: At least 20 interconnected locations
	spaceCount := 0
	for _, ent := range sc.World.Entities {
		if ent.Space != nil {
			spaceCount++
		}
	}

	if spaceCount < 20 {
		t.Fatalf("expected at least 20 spatial locations, got %d", spaceCount)
	}
	t.Logf("Total spatial locations verified: %d", spaceCount)

	// 2. Verify Key Sub-locations & Rooms in Nelly's Apartment
	if sc.Locations.LivingRoom == nil || sc.Locations.Kitchenette == nil ||
		sc.Locations.Balcony == nil || sc.Locations.Hallway == nil ||
		sc.Locations.GuestBathroom == nil || sc.Locations.NellyBedroom == nil ||
		sc.Locations.MasterBathroom == nil {
		t.Fatalf("missing essential apartment room topologies in Nelly's apartment")
	}

	// 3. Verify Props, Evidence & ACL
	if sc.Locations.AuditLetter == nil {
		t.Fatalf("expected housing audit letter prop to exist")
	}
	attr, ok := sc.Locations.AuditLetter.Attributes["Penalty Clause"]
	if !ok || attr.Visibility != sim.VisibilityPrivate || len(attr.Accessors) != 1 || attr.Accessors[0] != "nelly" {
		t.Fatalf("expected private ACL on Penalty Clause for nelly, got attr=%+v", attr)
	}

	// 4. Verify Nelly's Tier 2 Episodic Memory (David Arc + Audit Notice + Mia Scheme)
	nellyMind := sc.NellyMind.Mind
	nellyEpisodes := nellyMind.EpisodesSnapshot()
	if len(nellyEpisodes) < 6 {
		t.Fatalf("expected at least 6 authored episodes for Nelly, got %d", len(nellyEpisodes))
	}

	nellyEpIDs := make(map[string]bool)
	for _, ep := range nellyEpisodes {
		nellyEpIDs[ep.ID] = true
	}
	expectedNelly := []string{
		"ep-nelly-david-movein",
		"ep-nelly-david-market",
		"ep-nelly-david-distance",
		"ep-nelly-david-abandonment",
		"ep-nelly-audit-letter",
		"ep-nelly-mia-scheme",
	}
	for _, id := range expectedNelly {
		if !nellyEpIDs[id] {
			t.Errorf("missing expected Nelly episode: %s", id)
		}
	}

	// 5. Verify Zack's Tier 2 Episodic Memory (Housing Instability Arc)
	zackMind := sc.ZackMind.Mind
	zackEpisodes := zackMind.EpisodesSnapshot()
	if len(zackEpisodes) < 6 {
		t.Fatalf("expected at least 6 authored episodes for Zack, got %d", len(zackEpisodes))
	}

	zackEpIDs := make(map[string]bool)
	for _, ep := range zackEpisodes {
		zackEpIDs[ep.ID] = true
	}
	expectedZack := []string{
		"ep-zack-roommates-leave",
		"ep-zack-couch-surfing",
		"ep-zack-cybernet-cafe",
		"ep-zack-lecture-humiliation",
		"ep-zack-hotel-luxury",
		"ep-zack-mia-ambush",
	}
	for _, id := range expectedZack {
		if !zackEpIDs[id] {
			t.Errorf("missing expected Zack episode: %s", id)
		}
	}

	// 6. Verify Mia's Tier 2 Episodic Memory (Fixer Arc)
	miaMind := sc.MiaMind.Mind
	miaEpisodes := miaMind.EpisodesSnapshot()
	if len(miaEpisodes) < 3 {
		t.Fatalf("expected at least 3 authored episodes for Mia, got %d", len(miaEpisodes))
	}

	// 7. Verify Invariant: Zack and Nelly DO NOT KNOW EACH OTHER prior to introduction
	for _, ep := range zackEpisodes {
		for _, part := range ep.ParticipantIDs {
			if part == "nelly" {
				t.Fatalf("Zack cannot have Nelly as a past participant before the opening scene: found in %s", ep.ID)
			}
		}
	}
	for _, ep := range nellyEpisodes {
		for _, part := range ep.ParticipantIDs {
			if part == "zack" {
				t.Fatalf("Nelly cannot have Zack as a past participant before the opening scene: found in %s", ep.ID)
			}
		}
	}

	// 8. Verify Phantom Subjects and Tier 3 Dossiers
	nellyDavid, ok := nellyMind.GetDossier("david")
	if !ok || len(nellyDavid.Claims) == 0 {
		t.Fatalf("Nelly must have phantom dossier for runaway ex David with claims")
	}

	zackHarrison, ok := zackMind.GetDossier("professor-harrison")
	if !ok || len(zackHarrison.Claims) == 0 {
		t.Fatalf("Zack must have phantom dossier for Professor Harrison with claims")
	}

	// 9. Verify Graph Recall on Mention Cue
	// When Nelly thinks about "David" or "Uber", her abandonment episode must be recalled
	recallRes := nellyMind.Recall(memory.RecallCues{
		Mentions:    []string{"David", "Uber"},
		CurrentTick: 1,
		CurrentTime: time.Date(2026, 10, 16, 18, 35, 0, 0, time.UTC),
	})
	foundAbandonment := false
	for _, ep := range recallRes.SurfacedEpisodes {
		if ep.ID == "ep-nelly-david-abandonment" {
			foundAbandonment = true
			break
		}
	}
	if !foundAbandonment {
		t.Fatalf("expected mention of 'David' to recall ep-nelly-david-abandonment, got: %+v", recallRes.SurfacedEpisodes)
	}

	// When Zack thinks about "Harrison" or "lecture", his humiliation episode must be recalled
	zackRecall := zackMind.Recall(memory.RecallCues{
		Mentions:    []string{"Harrison", "lecture"},
		CurrentTick: 1,
		CurrentTime: time.Date(2026, 10, 16, 18, 35, 0, 0, time.UTC),
	})
	foundHumiliation := false
	for _, ep := range zackRecall.SurfacedEpisodes {
		if ep.ID == "ep-zack-lecture-humiliation" {
			foundHumiliation = true
			break
		}
	}
	if !foundHumiliation {
		t.Fatalf("expected mention of 'Harrison' to recall ep-zack-lecture-humiliation, got: %+v", zackRecall.SurfacedEpisodes)
	}
}
