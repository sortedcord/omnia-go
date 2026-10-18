package scenario

import (
	"testing"
	"time"

	"omnia.adityagupta.dev/pkg/memory"
	"omnia.adityagupta.dev/pkg/scenario/spec"
)

func TestBuildSimulationInstance(t *testing.T) {
	doc := validTestDocument()

	// Add an item and a portal to the test document to exercise all builders
	doc.Entities = append(doc.Entities,
		specEntity("balcony", "space", "Balcony", "", specSpace(true, false)),
		specEntity("door-balcony", "portal", "Balcony Door", "living-room", nil),
		specEntity("coffee-table", "item", "Coffee Table", "living-room", nil),
	)
	doc.Entities[len(doc.Entities)-2].Portal = &spec.PortalBlock{
		FromID: "living-room",
		ToID:   "balcony",
		Closed: true,
		Locked: true,
	}

	inst, err := Build(&doc, BuildOptions{
		Interactive: true,
		Player:      "zack-body",
	})
	if err != nil {
		t.Fatalf("expected clean build, got error: %v", err)
	}

	// 1. Verify Simulation Clock & Seed
	if inst.World.Clock.Cycle != 1 || inst.World.Clock.TickDuration != 20*time.Second {
		t.Fatalf("unexpected clock config: %+v", inst.World.Clock)
	}

	// 2. Verify Entity Tree & Portals
	livingRoom := inst.World.Entities["living-room"]
	door := inst.World.Entities["door-balcony"]
	zackBody := inst.World.Entities["zack-body"]

	if livingRoom == nil || livingRoom.Space == nil || !livingRoom.Space.Enterable {
		t.Fatalf("expected enterable living room space")
	}
	if door == nil || door.Portal == nil || !door.Portal.Closed || !door.Portal.Locked {
		t.Fatalf("expected closed & locked portal")
	}
	if zackBody == nil || zackBody.Parent != "living-room" {
		t.Fatalf("expected zack-body inside living-room")
	}

	// 3. Verify Actor Mind Seeding & Derived Links
	zackActor := inst.Actors["zack-body"]
	if zackActor == nil || zackActor.Mind == nil {
		t.Fatalf("expected zack actor and mind to exist")
	}

	// Check Dossier and Episode link
	doss, ok := zackActor.Mind.GetDossier("nelly")
	if !ok || len(doss.Claims) != 1 {
		t.Fatalf("expected nelly dossier with 1 claim, got: %+v", doss)
	}
	if len(doss.Links) == 0 {
		t.Fatalf("expected derived episode link in nelly dossier, got 0 links")
	}
	if doss.Links[0].EpisodeID != "ep-zack-1" {
		t.Fatalf("expected link to ep-zack-1, got %s", doss.Links[0].EpisodeID)
	}

	// Check graph recall works on mention of "Nelly"
	recall := zackActor.Mind.Recall(memory.RecallCues{
		Mentions:    []string{"Nelly"},
		CurrentTick: 1,
		CurrentTime: time.Date(2026, 10, 16, 18, 35, 0, 0, time.UTC),
	})
	if len(recall.SurfacedEpisodes) == 0 || recall.SurfacedEpisodes[0].ID != "ep-zack-1" {
		t.Fatalf("expected recall on mention 'Nelly' to surface ep-zack-1, got: %+v", recall.SurfacedEpisodes)
	}

	// 4. Verify Interactive Human Controller
	if inst.HumanCtrl == nil {
		t.Fatalf("expected HumanController to be attached for interactive player")
	}
	if inst.Player != "zack-body" {
		t.Fatalf("expected player entity to be zack-body, got %s", inst.Player)
	}
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
