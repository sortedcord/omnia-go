package scenario

import (
	"os"
	"strings"
	"testing"

	"omnia.adityagupta.dev/pkg/scenario/spec"
)

func validTestDocument() spec.Document {
	return spec.Document{
		Format: spec.Format{Name: "omnia.scenario", Version: "1.0"},
		Meta:   spec.Meta{ID: "scenario-test", Title: "Scenario Test"},
		Characters: []spec.Character{
			{ID: "alice", Name: "Alice", Personality: "Grounded"},
			{ID: "bob", Name: "Bob", Personality: "Considerate"},
		},
		Entities: []spec.Entity{
			{
				ID:    "room-a",
				Kind:  "space",
				Name:  "Room A",
				Space: &spec.SpaceBlock{Enterable: true, Open: true},
			},
			{
				ID:       "actor-a",
				Kind:     "actor",
				Name:     "Alice Body",
				ParentID: "room-a",
				Link:     &spec.Link{Relation: "in"},
				Actor:    &spec.ActorBlock{CharacterID: "alice"},
				Attributes: []spec.Attribute{
					{Key: "arousal", Value: "normal", Visibility: "private", Accessors: []string{"actor-a"}},
				},
			},
		},
		Minds: []spec.Mind{
			{
				OwnerID: "alice",
				Subjects: []spec.Subject{
					{
						ID:     "bob",
						Kind:   "person",
						Names:  []string{"Bob"},
						Origin: "authored",
						Bindings: []spec.Binding{
							{EntityID: "non-existent-world-entity", Mode: "identity", Confidence: 1.0},
						},
					},
					{
						ID:     "room-a",
						Kind:   "place",
						Names:  []string{"Room A"},
						Origin: "authored",
					},
				},
				Episodes: []spec.Episode{
					{
						ID:             "ep-alice-1",
						Start:          "2026-10-16T18:00:00Z",
						End:            "2026-10-16T18:20:00Z",
						Title:          "First Meeting",
						Summary:        "A quiet first meeting.",
						PlaceIDs:       []string{"room-a"},
						ParticipantIDs: []string{"bob"},
						Importance:     0.8,
						Beats: []spec.Beat{
							{Text: "We stood on the wooden floor.", SubjectIDs: []string{"bob"}},
						},
					},
				},
				Dossiers: []spec.Dossier{
					{
						SubjectID: "bob",
						Claims: []spec.Claim{
							{
								ID:         "c-alice-1",
								Kind:       "belief",
								Content:    "Bob is calm.",
								Salience:   0.8,
								Conviction: 0.9,
								Evidence:   []string{"ep-alice-1"},
							},
						},
					},
				},
			},
		},
		Starts: []spec.Start{
			{
				ID:       "start-opening",
				Name:     "Opening Scene",
				Clock:    spec.Clock{Cycle: 1, StartAt: "2026-10-16T18:35:00Z", TickSeconds: 20},
				PlayerID: "actor-a",
				Events: []spec.EventSchedule{
					{
						ID:         "ev-open",
						Tick:       1,
						LocationID: "room-a",
						Text:       "You arrive at the doorway.",
					},
				},
			},
		},
	}
}

func TestValidateValidDocument(t *testing.T) {
	doc := validTestDocument()
	rep := Validate(&doc, "")
	for _, iss := range rep.Issues {
		if iss.Severity == SeverityError {
			t.Fatalf("unexpected validation error: %s - %s", iss.Code, iss.Message)
		}
	}
	if !rep.Valid {
		t.Fatalf("expected valid report, got invalid")
	}

	// Memory Invariant I2 check:
	// Verify that binding to 'non-existent-world-entity' emitted SeverityInfo, NOT SeverityError!
	foundInfo := false
	for _, iss := range rep.Issues {
		if iss.Code == "UNBOUND_ENTITY" && iss.Severity == SeverityInfo {
			foundInfo = true
			break
		}
	}
	if !foundInfo {
		t.Fatalf("expected UNBOUND_ENTITY to emit SeverityInfo for fallible desync")
	}
}

func TestValidateNestedEnterableSpaceRejected(t *testing.T) {
	doc := validTestDocument()
	doc.Entities = append(doc.Entities, spec.Entity{
		ID:       "kitchen",
		Kind:     "space",
		Name:     "Kitchen",
		ParentID: "room-a",
		Space:    &spec.SpaceBlock{Enterable: true, Open: true},
	})
	rep := Validate(&doc, "")
	found := false
	for _, iss := range rep.Issues {
		if iss.Code == "NESTED_ENTERABLE_SPACE" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected NESTED_ENTERABLE_SPACE error")
	}
}

func TestValidateMindReferentialIntegrity(t *testing.T) {
	doc := validTestDocument()
	// Claim evidence references unknown episode
	doc.Minds[0].Dossiers[0].Claims[0].Evidence = []string{"non-existent-ep"}
	rep := Validate(&doc, "")
	found := false
	for _, iss := range rep.Issues {
		if iss.Code == "UNKNOWN_EPISODE" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected UNKNOWN_EPISODE error")
	}
}

func TestValidateAuthoringLintQuotedDialogue(t *testing.T) {
	doc := validTestDocument()
	doc.Starts[0].Events[0].Text = "Mia greets you: \"Hello there, slowpoke!\""
	rep := Validate(&doc, "")
	found := false
	for _, iss := range rep.Issues {
		if iss.Code == "LINT_SCRIPTED_DIALOGUE" && iss.Severity == SeverityWarning {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected LINT_SCRIPTED_DIALOGUE warning")
	}
	// Warnings do not invalidate document
	if !rep.Valid {
		t.Fatalf("expected document to remain valid despite warning")
	}
}

func TestValidateNoTick1EventRejected(t *testing.T) {
	doc := validTestDocument()
	doc.Starts[0].Events[0].Tick = 5 // no tick 1
	rep := Validate(&doc, "")
	found := false
	for _, iss := range rep.Issues {
		if iss.Code == "NO_TICK1_EVENT" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected NO_TICK1_EVENT error")
	}
}

func TestValidateEnumsRejected(t *testing.T) {
	doc := validTestDocument()
	doc.Entities = append(doc.Entities, spec.Entity{ID: "x1", Kind: "spcae", Name: "typo", ParentID: "room-a"})
	doc.Entities[1].Link.Relation = "inside"
	doc.Minds[0].Dossiers[0].Claims[0].Kind = "opinion"
	doc.Minds[0].Subjects[0].Bindings[0].Mode = "maybe"
	doc.Minds[0].Episodes[0].Precision = "week"

	rep := Validate(&doc, "")
	if rep.Valid {
		t.Fatalf("expected validation failure on invalid enums")
	}

	foundKind, foundRel, foundClaim, foundMode, foundPrec := false, false, false, false, false
	for _, iss := range rep.Issues {
		if iss.Code == "INVALID_ENUM" {
			switch iss.Pointer {
			case "/entities/2/kind":
				foundKind = true
			case "/entities/1/link/relation":
				foundRel = true
			case "/minds/0/dossiers/0/claims/0/kind":
				foundClaim = true
			case "/minds/0/subjects/0/bindings/0/mode":
				foundMode = true
			case "/minds/0/episodes/0/precision":
				foundPrec = true
			}
		}
	}
	if !foundKind || !foundRel || !foundClaim || !foundMode || !foundPrec {
		t.Fatalf("missing expected enum validation error: kind=%v rel=%v claim=%v mode=%v prec=%v",
			foundKind, foundRel, foundClaim, foundMode, foundPrec)
	}
}

func TestValidateStartClosureAndOverrides(t *testing.T) {
	doc := validTestDocument()
	doc.Starts[0].PresentIDs = []string{"actor-a"} // missing parent room-a
	doc.Starts[0].Overrides = []spec.EntityOverride{{EntityID: "ghost"}}
	doc.Starts[0].Minds = []spec.MindOverride{{OwnerID: "nobody", RetractClaimIDs: []string{"c-none"}}}

	rep := Validate(&doc, "start-opening")
	if rep.Valid {
		t.Fatalf("expected validation failure on bad start closure and overrides")
	}

	foundClosure, foundOverride, foundOwner := false, false, false
	for _, iss := range rep.Issues {
		if iss.Code == "MISSING_ANCESTOR_IN_PRESENT" {
			foundClosure = true
		}
		if iss.Code == "UNKNOWN_OVERRIDE_TARGET" {
			foundOverride = true
		}
		if iss.Code == "UNKNOWN_MIND_OWNER" {
			foundOwner = true
		}
	}
	if !foundClosure || !foundOverride || !foundOwner {
		t.Fatalf("missing expected start validation error: closure=%v override=%v owner=%v",
			foundClosure, foundOverride, foundOwner)
	}

	// Validate with non-existent start
	repUnknown := Validate(&doc, "does-not-exist")
	foundUnknownStart := false
	for _, iss := range repUnknown.Issues {
		if iss.Code == "START_NOT_FOUND" {
			foundUnknownStart = true
			break
		}
	}
	if !foundUnknownStart {
		t.Fatalf("expected START_NOT_FOUND error")
	}
}

func TestNoScenarioSpecificContentInPackageScenario(t *testing.T) {
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("failed reading pkg/scenario directory: %v", err)
	}

	scenarioTerms := []string{"accidental_arrangement", "accidental-arrangement", "zack", "nelly", "mia"}

	for _, fi := range files {
		if !strings.HasSuffix(fi.Name(), ".go") || fi.Name() == "validate_test.go" {
			continue // skip directory entries and this test file itself
		}

		content, err := os.ReadFile(fi.Name())
		if err != nil {
			t.Fatalf("failed reading file %s: %v", fi.Name(), err)
		}

		lower := strings.ToLower(string(content))
		for _, term := range scenarioTerms {
			if strings.Contains(lower, term) {
				t.Fatalf("acceptance criterion violated: found scenario-specific term %q in pkg/scenario/%s", term, fi.Name())
			}
		}
	}
}
