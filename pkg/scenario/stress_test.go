package scenario

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"omnia.adityagupta.dev/pkg/scenario/spec"
)

func generateStressDocument(charCount, episodesPerChar, entityCount int) *spec.Document {
	var chars []spec.Character
	var entities []spec.Entity
	var minds []spec.Mind

	// Create root space
	entities = append(entities, spec.Entity{
		ID:    "world-hub",
		Kind:  "space",
		Name:  "World Hub Space",
		Space: &spec.SpaceBlock{Enterable: true, Open: true},
	})

	for i := 0; i < charCount; i++ {
		cID := fmt.Sprintf("char-%d", i)
		chars = append(chars, spec.Character{
			ID:          cID,
			Name:        fmt.Sprintf("Character %d", i),
			Personality: "Analytical and observant",
			BornAt:      "2000-01-01T00:00:00Z",
		})

		// Actor body in world
		entities = append(entities, spec.Entity{
			ID:       fmt.Sprintf("actor-body-%d", i),
			Kind:     "actor",
			Name:     fmt.Sprintf("Body %d", i),
			ParentID: "world-hub",
			Link:     &spec.Link{Relation: "in"},
			Actor:    &spec.ActorBlock{CharacterID: cID},
		})

		// Mind
		var episodes []spec.Episode
		baseTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		for ep := 0; ep < episodesPerChar; ep++ {
			epStart := baseTime.Add(time.Duration(ep) * 24 * time.Hour)
			epEnd := epStart.Add(2 * time.Hour)
			episodes = append(episodes, spec.Episode{
				ID:         fmt.Sprintf("ep-%s-%d", cID, ep),
				Start:      epStart.Format(time.RFC3339),
				End:        epEnd.Format(time.RFC3339),
				Title:      fmt.Sprintf("Episode %d for %s", ep, cID),
				Summary:    "A detailed memory of an event.",
				Importance: 0.50,
				Beats: []spec.Beat{
					{Text: "A beat in this memory occurred."},
				},
			})
		}

		minds = append(minds, spec.Mind{
			OwnerID:  cID,
			Episodes: episodes,
			Subjects: []spec.Subject{
				{ID: "self", Kind: "person", Names: []string{"Me"}, Origin: "authored"},
			},
			Dossiers: []spec.Dossier{
				{
					SubjectID: "self",
					Claims: []spec.Claim{
						{ID: fmt.Sprintf("c-self-%d", i), Kind: "belief", Content: "I am ready.", Salience: 0.8, Conviction: 0.9},
					},
				},
			},
		})
	}

	for i := len(entities); i < entityCount; i++ {
		entities = append(entities, spec.Entity{
			ID:       fmt.Sprintf("item-%d", i),
			Kind:     "item",
			Name:     fmt.Sprintf("Item %d", i),
			ParentID: "world-hub",
			Link:     &spec.Link{Relation: "on"},
			Attributes: []spec.Attribute{
				{Key: "weight", Value: "light", Visibility: "public"},
				{Key: "material", Value: "wood", Visibility: "public"},
			},
		})
	}

	doc := &spec.Document{
		Format:     spec.Format{Name: "omnia.scenario", Version: "1.0"},
		Meta:       spec.Meta{ID: "stress-scenario", Title: "Stress Test Scenario"},
		Characters: chars,
		Entities:   entities,
		Minds:      minds,
		Starts: []spec.Start{
			{
				ID:       "start-stress",
				Name:     "Stress Start",
				Clock:    spec.Clock{Cycle: 1, StartAt: "2026-10-16T18:00:00Z", TickSeconds: 20},
				PlayerID: "actor-body-0",
				Events: []spec.EventSchedule{
					{ID: "ev-stress", Tick: 1, LocationID: "world-hub", Text: "Simulation begins."},
				},
			},
		},
	}

	return doc
}

func TestStressScale(t *testing.T) {
	// 20 characters x 100 episodes = 2,000 episodes; 1,000 entities
	doc := generateStressDocument(20, 100, 1000)

	start := time.Now()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	decodeStart := time.Now()
	decoded, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	decodeDur := time.Since(decodeStart)

	validateStart := time.Now()
	rep := Validate(decoded, "")
	if !rep.Valid {
		t.Fatalf("validation failed: %+v", rep.Issues)
	}
	validateDur := time.Since(validateStart)

	buildStart := time.Now()
	inst, err := Build(decoded, BuildOptions{Interactive: false})
	if err != nil {
		t.Fatalf("build error: %v", err)
	}
	buildDur := time.Since(buildStart)
	totalDur := time.Since(start)

	t.Logf("Stress Test Metrics (Payload size: %.2f MB):", float64(len(data))/(1024*1024))
	t.Logf("  Decode:   %v", decodeDur)
	t.Logf("  Validate: %v", validateDur)
	t.Logf("  Build:    %v", buildDur)
	t.Logf("  Total:    %v (Budget < 2.0s)", totalDur)

	if totalDur > 2*time.Second {
		t.Fatalf("stress load exceeded 2.0s performance budget: %v", totalDur)
	}
	if len(inst.World.Entities) != 1000 {
		t.Fatalf("expected 1000 entities in built world, got %d", len(inst.World.Entities))
	}
}
