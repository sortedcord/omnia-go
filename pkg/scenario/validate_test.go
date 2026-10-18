package scenario

import (
	"testing"

	"omnia.adityagupta.dev/pkg/scenario/spec"
)

func validTestDocument() spec.Document {
	return spec.Document{
		Format: spec.Format{Name: "omnia.scenario", Version: "1.0"},
		Meta:   spec.Meta{ID: "scenario-test", Title: "Scenario Test"},
		Characters: []spec.Character{
			{ID: "zack", Name: "Zack", Personality: "Grounded"},
			{ID: "nelly", Name: "Nelly", Personality: "Considerate"},
		},
		Entities: []spec.Entity{
			{
				ID:    "living-room",
				Kind:  "space",
				Name:  "Living Room",
				Space: &spec.SpaceBlock{Enterable: true, Open: true},
			},
			{
				ID:       "zack-body",
				Kind:     "actor",
				Name:     "Zack",
				ParentID: "living-room",
				Link:     &spec.Link{Relation: "in"},
				Actor:    &spec.ActorBlock{CharacterID: "zack"},
				Attributes: []spec.Attribute{
					{Key: "arousal", Value: "normal", Visibility: "private", Accessors: []string{"zack-body"}},
				},
			},
		},
		Minds: []spec.Mind{
			{
				OwnerID: "zack",
				Subjects: []spec.Subject{
					{
						ID:     "nelly",
						Kind:   "person",
						Names:  []string{"Nelly"},
						Origin: "authored",
						Bindings: []spec.Binding{
							{EntityID: "non-existent-world-entity", Mode: "identity", Confidence: 1.0},
						},
					},
					{
						ID:     "living-room",
						Kind:   "place",
						Names:  []string{"Living Room"},
						Origin: "authored",
					},
				},
				Episodes: []spec.Episode{
					{
						ID:             "ep-zack-1",
						Start:          "2026-10-16T18:00:00Z",
						End:            "2026-10-16T18:20:00Z",
						Title:          "First Meeting",
						Summary:        "A quiet first meeting.",
						PlaceIDs:       []string{"living-room"},
						ParticipantIDs: []string{"nelly"},
						Importance:     0.8,
						Beats: []spec.Beat{
							{Text: "We stood on the wooden floor.", SubjectIDs: []string{"nelly"}},
						},
					},
				},
				Dossiers: []spec.Dossier{
					{
						SubjectID: "nelly",
						Claims: []spec.Claim{
							{
								ID:         "c-zack-1",
								Kind:       "belief",
								Content:    "Nelly is anxious.",
								Salience:   0.8,
								Conviction: 0.9,
								Evidence:   []string{"ep-zack-1"},
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
				PlayerID: "zack-body",
				Events: []spec.EventSchedule{
					{
						ID:         "ev-open",
						Tick:       1,
						LocationID: "living-room",
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
		ParentID: "living-room",
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
