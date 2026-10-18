package scenario

import (
	"bytes"
	"strings"
	"testing"
)

const minimalValidScenarioJSON = `{
  "$schema": "./schema/omnia.scenario.v1.json",
  "format": { "name": "omnia.scenario", "version": "1.0" },
  "meta": { "id": "test-scenario", "title": "Test Scenario" },
  "characters": [
    { "id": "alice", "name": "Alice", "personality": "Curious" }
  ],
  "entities": [
    { "id": "room-a", "kind": "space", "name": "Room A" }
  ],
  "minds": [
    {
      "owner_id": "alice",
      "subjects": [ { "id": "self", "kind": "person", "names": ["Alice"], "origin": "authored" } ],
      "dossiers": [ { "subject_id": "self", "claims": [] } ],
      "episodes": []
    }
  ],
  "starts": [
    {
      "id": "start-1",
      "name": "Opening",
      "clock": { "cycle": 1, "start_at": "2026-10-16T18:00:00Z", "tick_seconds": 20 }
    }
  ],
  "editor": { "canvas": { "zoom": 1.0 } }
}`

func TestDecodeValidDocument(t *testing.T) {
	doc, err := Decode(strings.NewReader(minimalValidScenarioJSON))
	if err != nil {
		t.Fatalf("expected valid decode, got error: %v", err)
	}
	if doc.Meta.ID != "test-scenario" {
		t.Fatalf("expected meta.id 'test-scenario', got %q", doc.Meta.ID)
	}
	if doc.Format.Version != "1.0" {
		t.Fatalf("expected format 1.0, got %q", doc.Format.Version)
	}
	if len(doc.Characters) != 1 || doc.Characters[0].ID != "alice" {
		t.Fatalf("expected character alice, got %+v", doc.Characters)
	}
	if len(doc.Editor) == 0 {
		t.Fatalf("expected editor object to pass through")
	}
}

func TestDecodeSizeLimit(t *testing.T) {
	huge := strings.Repeat(" ", 200) + minimalValidScenarioJSON
	_, err := Decode(strings.NewReader(huge), DecodeOptions{MaxBytes: 100})
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum allowed size") {
		t.Fatalf("expected size limit error, got: %v", err)
	}
}

func TestDecodeDuplicateKeys(t *testing.T) {
	dupJSON := `{
  "format": { "name": "omnia.scenario", "version": "1.0" },
  "meta": { "id": "test-scenario", "title": "Test Scenario", "title": "Second Title" },
  "characters": [],
  "entities": [],
  "minds": [],
  "starts": [{ "id": "s1", "name": "S1", "clock": { "cycle": 1, "start_at": "2026-10-16T18:00:00Z", "tick_seconds": 20 } }]
}`
	_, err := Decode(strings.NewReader(dupJSON))
	if err == nil || !strings.Contains(err.Error(), "duplicate key \"title\" detected") {
		t.Fatalf("expected duplicate key error, got: %v", err)
	}
}

func TestDecodeUnsupportedFormatName(t *testing.T) {
	badName := bytes.ReplaceAll([]byte(minimalValidScenarioJSON), []byte(`"omnia.scenario"`), []byte(`"other.format"`))
	_, err := Decode(bytes.NewReader(badName))
	if err == nil || !strings.Contains(err.Error(), "unsupported format name") {
		t.Fatalf("expected unsupported format name error, got: %v", err)
	}
}

func TestDecodeUnsupportedMajorVersion(t *testing.T) {
	badMajor := bytes.ReplaceAll([]byte(minimalValidScenarioJSON), []byte(`"1.0"`), []byte(`"2.0"`))
	_, err := Decode(bytes.NewReader(badMajor))
	if err == nil || !strings.Contains(err.Error(), "unsupported format major version 2") {
		t.Fatalf("expected unsupported major version error, got: %v", err)
	}
}

func TestDecodeNewerMinorVersion(t *testing.T) {
	badMinor := bytes.ReplaceAll([]byte(minimalValidScenarioJSON), []byte(`"1.0"`), []byte(`"1.3"`))
	_, err := Decode(bytes.NewReader(badMinor))
	if err == nil || !strings.Contains(err.Error(), "requires format 1.3; engine supports <= 1.0") {
		t.Fatalf("expected newer minor version error, got: %v", err)
	}
}

func TestDecodeUnknownFieldsRejected(t *testing.T) {
	unknownField := `{
  "format": { "name": "omnia.scenario", "version": "1.0" },
  "meta": { "id": "test-scenario", "title": "Test Scenario" },
  "unknown_top_level_field": "disallowed",
  "characters": [],
  "entities": [],
  "minds": [],
  "starts": [{ "id": "s1", "name": "S1", "clock": { "cycle": 1, "start_at": "2026-10-16T18:00:00Z", "tick_seconds": 20 } }]
}`
	_, err := Decode(strings.NewReader(unknownField))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected strict decode error on unknown field, got: %v", err)
	}
}
