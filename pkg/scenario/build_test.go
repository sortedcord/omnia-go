package scenario

import (
	"context"
	"testing"
	"time"

	"omnia.adityagupta.dev/pkg/llm"
	"omnia.adityagupta.dev/pkg/memory"
	"omnia.adityagupta.dev/pkg/scenario/spec"
)

func TestBuildSimulationInstance(t *testing.T) {
	doc := validTestDocument()

	// Add an item and a portal to the test document to exercise all builders
	doc.Entities = append(doc.Entities,
		specEntity("balcony", "space", "Balcony", "", specSpace(true, false)),
		specEntity("door-balcony", "portal", "Balcony Door", "room-a", nil),
		specEntity("coffee-table", "item", "Coffee Table", "room-a", nil),
	)
	doc.Entities[len(doc.Entities)-2].Portal = &spec.PortalBlock{
		FromID: "room-a",
		ToID:   "balcony",
		Closed: true,
		Locked: true,
	}

	inst, err := Build(&doc, BuildOptions{
		Interactive: true,
		Player:      "actor-a",
	})
	if err != nil {
		t.Fatalf("expected clean build, got error: %v", err)
	}

	// 1. Verify Simulation Clock & Seed
	if inst.World.Clock.Cycle != 1 || inst.World.Clock.TickDuration != 20*time.Second {
		t.Fatalf("unexpected clock config: %+v", inst.World.Clock)
	}

	// 2. Verify Entity Tree & Portals
	roomA := inst.World.Entities["room-a"]
	door := inst.World.Entities["door-balcony"]
	actorA := inst.World.Entities["actor-a"]

	if roomA == nil || roomA.Space == nil || !roomA.Space.Enterable {
		t.Fatalf("expected enterable room-a space")
	}
	if door == nil || door.Portal == nil || !door.Portal.Closed || !door.Portal.Locked {
		t.Fatalf("expected closed & locked portal")
	}
	if actorA == nil || actorA.Parent != "room-a" {
		t.Fatalf("expected actor-a inside room-a")
	}

	// 3. Verify Actor Mind Seeding & Derived Links
	aliceActor := inst.Actors["actor-a"]
	if aliceActor == nil || aliceActor.Mind == nil {
		t.Fatalf("expected alice actor and mind to exist")
	}

	// Check Dossier and Episode link
	doss, ok := aliceActor.Mind.GetDossier("bob")
	if !ok || len(doss.Claims) != 1 {
		t.Fatalf("expected bob dossier with 1 claim, got: %+v", doss)
	}
	if len(doss.Links) == 0 {
		t.Fatalf("expected derived episode link in bob dossier, got 0 links")
	}
	if doss.Links[0].EpisodeID != "ep-alice-1" {
		t.Fatalf("expected link to ep-alice-1, got %s", doss.Links[0].EpisodeID)
	}

	// Check graph recall works on mention of "Bob"
	recall := aliceActor.Mind.Recall(memory.RecallCues{
		Mentions:    []string{"Bob"},
		CurrentTick: 1,
		CurrentTime: time.Date(2026, 10, 16, 18, 35, 0, 0, time.UTC),
	})
	if len(recall.SurfacedEpisodes) == 0 || recall.SurfacedEpisodes[0].ID != "ep-alice-1" {
		t.Fatalf("expected recall on mention 'Bob' to surface ep-alice-1, got: %+v", recall.SurfacedEpisodes)
	}

	// 4. Verify Interactive Human Controller
	if inst.HumanCtrl == nil {
		t.Fatalf("expected HumanController to be attached for interactive player")
	}
	if inst.Player != "actor-a" {
		t.Fatalf("expected player entity to be actor-a, got %s", inst.Player)
	}
}

func TestBuildRetractClaims(t *testing.T) {
	doc := validTestDocument()
	doc.Starts[0].Minds = []spec.MindOverride{
		{
			OwnerID:         "alice",
			RetractClaimIDs: []string{"c-alice-1"},
		},
	}

	inst, err := Build(&doc, BuildOptions{
		Client: &fakeClient{},
	})
	if err != nil {
		t.Fatalf("expected build to succeed: %v", err)
	}

	doss, ok := inst.Actors["actor-a"].Mind.GetDossier("bob")
	if !ok {
		t.Fatalf("missing bob dossier")
	}
	if len(doss.Claims) != 0 {
		t.Fatalf("expected claim c-alice-1 to be retracted, but %d active claims remain", len(doss.Claims))
	}
	if len(doss.ArchivedClaims) != 1 {
		t.Fatalf("expected retracted claim to be in ArchivedClaims, got %d", len(doss.ArchivedClaims))
	}
}

type fakeClient struct{}

func (f *fakeClient) ChatCompletion(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{}, nil
}

func specEntity(id, kind, name, parentID string, sp *spec.SpaceBlock) spec.Entity {
	return spec.Entity{
		ID:       id,
		Kind:     kind,
		Name:     name,
		ParentID: parentID,
		Space:    sp,
	}
}

func specSpace(enterable, transparent bool) *spec.SpaceBlock {
	return &spec.SpaceBlock{
		Enterable:   enterable,
		Open:        true,
		Transparent: transparent,
	}
}
