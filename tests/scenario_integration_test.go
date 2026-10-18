package tests

import (
	"os"
	"testing"
	"time"

	"omnia.adityagupta.dev/pkg/memory"
	"omnia.adityagupta.dev/pkg/scenario"
	"omnia.adityagupta.dev/pkg/sim"
)

func TestAccidentalArrangementScenarioLoading(t *testing.T) {
	if os.Getenv("OMNIA_API_KEY") == "" && os.Getenv("OPENAI_API_KEY") == "" {
		_ = os.Setenv("OMNIA_API_KEY", "dummy-key-for-offline-test")
	}

	f, err := os.Open("../scenarios/accidental_arrangement.omnia.json")
	if err != nil {
		t.Fatalf("failed opening scenario file: %v", err)
	}
	defer f.Close()

	doc, err := scenario.Decode(f)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	report := scenario.Validate(doc, "")
	if !report.Valid {
		t.Fatalf("validation failed: %+v", report.Issues)
	}

	inst, err := scenario.Build(doc, scenario.BuildOptions{
		Interactive: false,
	})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	// 1. Verify Topology & Entities
	if len(inst.World.Entities) < 23 {
		t.Fatalf("expected at least 23 entities in loaded world, got %d", len(inst.World.Entities))
	}

	// 2. Verify Actors
	if len(inst.Actors) != 3 {
		t.Fatalf("expected 3 actors (zack, nelly, mia), got %d", len(inst.Actors))
	}
	for _, id := range []string{"zack", "nelly", "mia"} {
		if act, ok := inst.Actors[sim.EntityID(id)]; !ok || act.Mind == nil {
			t.Fatalf("missing actor or mind for %q", id)
		}
	}

	// 3. Verify Recall Probes
	nellyMind := inst.Actors[sim.EntityID("nelly")].Mind
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
		t.Fatalf("expected mention of 'David' to recall ep-nelly-david-abandonment")
	}

	zackMind := inst.Actors[sim.EntityID("zack")].Mind
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
		t.Fatalf("expected mention of 'Harrison' to recall ep-zack-lecture-humiliation")
	}
}
