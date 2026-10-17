package sim

import (
	"context"
	"errors"
	"strings"
	"testing"

	"omnia.adityagupta.dev/pkg/actor"
)

// fakeController returns a fixed action without any LLM call.
type fakeController struct{ action actor.Action }

func (f fakeController) Decide(_ context.Context, _ *actor.Actor, _ Turn) (*actor.Reaction, error) {
	return &actor.Reaction{Action: f.action}, nil
}

type fakeArbiter struct{ ruling SceneRuling }

func (f fakeArbiter) ResolveScene(_ context.Context, adj SceneAdjudication) (*SceneRuling, error) {
	r := f.ruling
	r.SceneID = adj.SceneID
	return &r, nil
}

type failingBroadcaster struct{}

func (failingBroadcaster) Broadcast(context.Context, BroadcastInput) (BroadcastResult, error) {
	return BroadcastResult{}, errors.New("offline")
}

// offlineWorld builds a world with no network-backed components.
func offlineWorld() *Simulation {
	w := New(Clock{TickCount: 1}, WithSeed(7))
	w.Arbiter, w.Broadcaster, w.Distiller, w.Summarizer = nil, nil, nil, nil
	return w
}

func spawnActor(t *testing.T, w *Simulation, id EntityID, name string, room EntityID, c Controller) {
	t.Helper()
	e, err := w.SpawnActor(id, actor.New(actor.Profile{ID: string(id), Name: name}), room, "")
	if err != nil {
		t.Fatal(err)
	}
	e.Controller = c
}

func TestResolveTargetsPrefersExactMatch(t *testing.T) {
	view := []EntityRef{
		{ID: "living-room", Name: "Nelly's Apartment - Living Room"},
		{ID: "nelly", Name: "Nelly"},
	}
	got := ResolveTargets([]string{"Nelly"}, view)
	if len(got) != 1 || got[0] != "nelly" {
		t.Fatalf("want [nelly], got %v", got)
	}
}

func TestResolveTargetsDropsAmbiguousPartial(t *testing.T) {
	view := []EntityRef{{ID: "a", Name: "Red Mug"}, {ID: "b", Name: "Blue Mug"}}
	if got := ResolveTargets([]string{"mug"}, view); len(got) != 0 {
		t.Fatalf("ambiguous partial should resolve to nothing, got %v", got)
	}
	if got := ResolveTargets([]string{"red"}, view); len(got) != 1 || got[0] != "a" {
		t.Fatalf("unique partial should resolve, got %v", got)
	}
}

func TestEmptySpacesDoNotMintScenes(t *testing.T) {
	w := offlineWorld()
	a := w.SpawnSpace("room-a", "Room A", "", true)
	w.SpawnSpace("room-b", "Room B", "", true)
	spawnActor(t, w, "x", "X", a.ID, nil)
	for tick := 1; tick <= 3; tick++ {
		scenes := w.BuildScenes(tick)
		if len(scenes) != 1 {
			t.Fatalf("tick %d: want 1 scene, got %d", tick, len(scenes))
		}
		if _, ok := scenes["scene-1"]; !ok {
			t.Fatalf("tick %d: scene ID changed: %v", tick, scenes)
		}
	}
}

func TestSplitKeepsIDWithLargerGroup(t *testing.T) {
	w := offlineWorld()
	a := w.SpawnSpace("a", "A", "", true)
	b := w.SpawnSpace("b", "B", "", true)
	door, _ := w.SpawnPortal("door", "Door", a.ID, b.ID, "")
	spawnActor(t, w, "z1", "Z1", a.ID, nil)
	spawnActor(t, w, "z2", "Z2", a.ID, nil)
	spawnActor(t, w, "m", "M", b.ID, nil)
	w.BuildScenes(1)
	door.Portal.Closed = true
	scenes := w.BuildScenes(2)
	original := scenes["scene-1"]
	if original == nil || len(original.ActorIDs) != 2 {
		t.Fatalf("larger group should keep scene-1, got %+v", scenes)
	}
}

func TestBondDissolvesAfterQuietTick(t *testing.T) {
	w := offlineWorld()
	a := w.SpawnSpace("a", "A", "", true)
	b := w.SpawnSpace("b", "B", "", true)
	door, _ := w.SpawnPortal("door", "Door", a.ID, b.ID, "")
	door.Portal.Closed = true
	spawnActor(t, w, "x", "X", a.ID, nil)
	spawnActor(t, w, "y", "Y", b.ID, nil)
	scenes := w.BuildScenes(1)
	if len(scenes) != 2 {
		t.Fatalf("closed door should split scenes, got %d", len(scenes))
	}
	sx, _ := w.SceneForActor("x")
	sy, _ := w.SceneForActor("y")
	w.mu.Lock()
	w.bondScenesLocked("door", sx.ID, sy.ID)
	w.mu.Unlock()
	if got := len(w.BuildScenes(2)); got != 1 {
		t.Fatalf("bond should merge scenes on the next tick, got %d", got)
	}
	if got := len(w.BuildScenes(3)); got != 2 {
		t.Fatalf("bond should dissolve when no one touches the door, got %d", got)
	}
}

func TestCrossSceneConflictOldestWins(t *testing.T) {
	w := offlineWorld()
	a := w.SpawnSpace("a", "A", "", true)
	b := w.SpawnSpace("b", "B", "", true)
	door, _ := w.SpawnPortal("door", "Door", a.ID, b.ID, "")
	door.Portal.Closed = true
	spawnActor(t, w, "x", "X", a.ID, nil)
	w.BuildScenes(1)
	spawnActor(t, w, "y", "Y", b.ID, nil) // newer scene
	w.BuildScenes(2)
	sx, _ := w.SceneForActor("x")
	sy, _ := w.SceneForActor("y")

	open, shut := false, true
	older := &SceneRuling{Effects: []Effect{{Type: EffectSetPortal, EntityID: "door", Closed: &open}}}
	newer := &SceneRuling{Effects: []Effect{{Type: EffectSetPortal, EntityID: "door", Closed: &shut}}}
	mutated := map[EntityID]SceneID{}
	w.mu.Lock()
	w.applySceneEffectsLocked(sx, older, mutated)
	w.applySceneEffectsLocked(sy, newer, mutated)
	w.mu.Unlock()

	if door.Portal.Closed {
		t.Fatal("older scene's change should win")
	}
	if len(newer.RejectedEffects) != 1 || !strings.Contains(newer.RejectedEffects[0], "conflict") {
		t.Fatalf("newer scene's change should be rejected as a conflict: %v", newer.RejectedEffects)
	}
	if len(w.bonds) != 1 {
		t.Fatalf("conflict should bond scenes, got %d bonds", len(w.bonds))
	}
}

func TestAttributeChangesInDifferentScenesDoNotBond(t *testing.T) {
	w := offlineWorld()
	a := w.SpawnSpace("a", "A", "", true)
	b := w.SpawnSpace("b", "B", "", true)
	spawnActor(t, w, "x", "X", a.ID, nil)
	spawnActor(t, w, "y", "Y", b.ID, nil)
	w.BuildScenes(1)
	sx, _ := w.SceneForActor("x")
	sy, _ := w.SceneForActor("y")
	mutated := map[EntityID]SceneID{}
	w.mu.Lock()
	w.applySceneEffectsLocked(sx, &SceneRuling{Effects: []Effect{{Type: EffectSetAttribute, EntityID: "x", Key: "mood", Value: "calm"}}}, mutated)
	w.applySceneEffectsLocked(sy, &SceneRuling{Effects: []Effect{{Type: EffectSetAttribute, EntityID: "y", Key: "mood", Value: "calm"}}}, mutated)
	w.mu.Unlock()
	if len(w.bonds) != 0 {
		t.Fatalf("unrelated attribute changes must not bond scenes, got %d", len(w.bonds))
	}
}

func TestSceneCannotMutateOutsideItsSpaces(t *testing.T) {
	w := offlineWorld()
	a := w.SpawnSpace("a", "A", "", true)
	b := w.SpawnSpace("b", "B", "", true)
	spawnActor(t, w, "x", "X", a.ID, nil)
	spawnActor(t, w, "y", "Y", b.ID, nil)
	w.BuildScenes(1)
	sx, _ := w.SceneForActor("x")
	r := &SceneRuling{Effects: []Effect{{Type: EffectSetAttribute, EntityID: "y", Key: "mood", Value: "hacked"}}}
	w.mu.Lock()
	w.applySceneEffectsLocked(sx, r, map[EntityID]SceneID{})
	w.mu.Unlock()
	if len(r.RejectedEffects) != 1 {
		t.Fatalf("out-of-scene effect should be rejected: %v", r.RejectedEffects)
	}
}

func TestDeterministicRollIndependentOfActorCount(t *testing.T) {
	a := deterministicRoll(1, 5, "nelly", "luck")
	if a != deterministicRoll(1, 5, "nelly", "luck") {
		t.Fatal("roll must be deterministic")
	}
	if a == deterministicRoll(1, 5, "nelly", "spontaneity") {
		t.Fatal("salts must give independent streams")
	}
	if a < 0 || a >= 1 {
		t.Fatalf("roll out of range: %v", a)
	}
}

func TestStepWithoutBroadcasterDoesNotLeakResolution(t *testing.T) {
	w := offlineWorld()
	room := w.SpawnSpace("room", "Room", "", true)
	secret := "THE SAFE BEHIND THE PAINTING IS OPEN"
	w.Arbiter = fakeArbiter{ruling: SceneRuling{Resolution: secret}}
	spawnActor(t, w, "x", "X", room.ID, fakeController{action: actor.Action{Type: actor.Wait, Action: "wait"}})
	spawnActor(t, w, "y", "Y", room.ID, nil)
	w.DeliverDirectEvent("x", "a noise")

	if _, err := w.Step(context.Background()); err != nil {
		t.Fatalf("nil broadcaster must not crash: %v", err)
	}
	for _, ev := range w.inbox["x"] {
		if strings.Contains(ev.Description, secret) {
			t.Fatalf("raw resolution leaked into perception: %q", ev.Description)
		}
	}

	w.Broadcaster = failingBroadcaster{}
	w.DeliverDirectEvent("x", "another noise")
	if _, err := w.Step(context.Background()); err != nil {
		t.Fatalf("failing broadcaster must not crash: %v", err)
	}
	for _, ev := range w.inbox["x"] {
		if strings.Contains(ev.Description, secret) {
			t.Fatalf("raw resolution leaked after broadcaster failure: %q", ev.Description)
		}
	}
}

func TestMultiSpaceSceneEventTaggedWithAllSpaces(t *testing.T) {
	w := offlineWorld()
	a := w.SpawnSpace("a", "A", "", true)
	b := w.SpawnSpace("b", "B", "", true)
	_, _ = w.SpawnPortal("door", "Door", a.ID, b.ID, "")
	w.Arbiter = fakeArbiter{ruling: SceneRuling{Resolution: "talk"}}
	spawnActor(t, w, "x", "X", a.ID, fakeController{action: actor.Action{Type: actor.Wait, Action: "wait"}})
	spawnActor(t, w, "y", "Y", b.ID, nil)
	w.DeliverDirectEvent("x", "hi")
	if _, err := w.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	ev := w.EventLog[len(w.EventLog)-1]
	if ev.LocationID != "" || len(ev.SpaceIDs) != 2 {
		t.Fatalf("multi-space event should list both spaces and no single location: %+v", ev)
	}
}

func TestTargetedSpontaneityUsesLastTickTargets(t *testing.T) {
	w := offlineWorld()
	room := w.SpawnSpace("room", "Room", "", true)
	spawnActor(t, w, "x", "X", room.ID, nil)
	base := w.ComputeSpontaneity("x", 1, &Event{})
	targeted := w.ComputeSpontaneity("x", 1, &Event{Targets: []EntityID{"x"}})
	if diff := targeted - base; diff < 0.24 || diff > 0.26 {
		t.Fatalf("being targeted should add 0.25, got %v", diff)
	}
}

func TestOrderEffectsFollowsExecutionOrder(t *testing.T) {
	r := &SceneRuling{
		ExecutionOrder: []EntityID{"mia", "zack"},
		Effects: []Effect{
			{Type: EffectSetAttribute, EntityID: "zack", Key: "mood"},
			{Type: EffectSetAttribute, EntityID: "mia", Key: "mood"},
		},
	}
	orderEffects(r)
	if r.Effects[0].EntityID != "mia" {
		t.Fatalf("mia acted first, effects: %+v", r.Effects)
	}
}

func TestAttributeACL(t *testing.T) {
	attrs := NewAttributes()

	// 1. Public attribute: everyone can observe, accessors cleared out
	_ = attrs.SetWithVisibility("reputation", "Honest", VisibilityPublic, []EntityID{"zack"})
	if attr, ok := attrs["reputation"]; !ok {
		t.Fatal("reputation not found")
	} else if len(attr.Accessors) != 0 {
		t.Fatalf("public attribute must clear accessors, got %v", attr.Accessors)
	} else if !attr.CanAccess("random-onlooker") {
		t.Fatal("public attribute should be accessible to random onlookers")
	}

	// 2. Private attribute with whitelist accessors
	_ = attrs.SetWithVisibility("hidden_weapon", "Knife in boot", VisibilityPrivate, []EntityID{"zack", "ally"})
	if attr, ok := attrs["hidden_weapon"]; !ok {
		t.Fatal("hidden_weapon not found")
	} else {
		if !attr.CanAccess("zack") {
			t.Fatal("zack should have access to hidden_weapon")
		}
		if !attr.CanAccess("ally") {
			t.Fatal("ally should have access to hidden_weapon")
		}
		if attr.CanAccess("guard") {
			t.Fatal("guard should NOT have access to hidden_weapon")
		}
	}

	// 3. Private attribute with EMPTY accessors: even owner cannot observe (Arbiter only!)
	_ = attrs.SetWithVisibility("latent_infection", "Stage 2 pathogen", VisibilityPrivate, nil)
	if attr, ok := attrs["latent_infection"]; !ok {
		t.Fatal("latent_infection not found")
	} else {
		if attr.CanAccess("zack") {
			t.Fatal("owner zack must NOT have access to private attribute with empty accessors")
		}
		if attr.CanAccess("doctor") {
			t.Fatal("doctor must NOT have access to private attribute with empty accessors")
		}
	}

	// 4. Test AccessibleTo map
	accessible := attrs.AccessibleTo("zack")
	if accessible["reputation"] != "Honest" {
		t.Fatal("zack should see reputation")
	}
	if accessible["hidden_weapon"] != "Knife in boot" {
		t.Fatal("zack should see hidden_weapon")
	}
	if _, seen := accessible["latent_infection"]; seen {
		t.Fatal("zack must not see latent_infection")
	}
}

func TestAttributeMutationOnProps(t *testing.T) {
	w := offlineWorld()
	room := w.SpawnSpace("room", "Room", "", true)
	door, _ := w.SpawnPortal("door", "Door", room.ID, "room-2", "")
	chair, _ := w.SpawnEntity("chair", "Wooden Chair", "A chair", room.ID, Link{Relation: RelOn})
	sx := &Scene{ID: "scene-1", SpaceIDs: []EntityID{room.ID}}

	// Arbiter sets attribute on a non-mind entity (a door portal and a chair!)
	r := &SceneRuling{
		Effects: []Effect{
			{Type: EffectSetAttribute, EntityID: door.ID, Key: "material", Value: "Reinforced Titanium"},
			{Type: EffectSetAttribute, EntityID: chair.ID, Key: "wobbly", Value: "true"},
		},
	}
	mutated := map[EntityID]SceneID{}
	w.mu.Lock()
	w.applySceneEffectsLocked(sx, r, mutated)
	w.mu.Unlock()

	if len(r.RejectedEffects) != 0 {
		t.Fatalf("mutations on props should succeed: %v", r.RejectedEffects)
	}
	if val, ok := door.Attributes.Get("material"); !ok || val != "Reinforced Titanium" {
		t.Fatalf("door material attribute failed: %s, %v", val, ok)
	}
	if val, ok := chair.Attributes.Get("wobbly"); !ok || val != "true" {
		t.Fatalf("chair wobbly attribute failed: %s, %v", val, ok)
	}
}
