package spec_test

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/invopop/jsonschema"
	"omnia.adityagupta.dev/pkg/memory"
	"omnia.adityagupta.dev/pkg/scenario/spec"
	"omnia.adityagupta.dev/pkg/sim"
)

// TestEnumParityWithEngine verifies that all enums declared in the scenario spec
// precisely match their authoritative Go constants in pkg/sim and pkg/memory.
func TestEnumParityWithEngine(t *testing.T) {
	// 1. Spatial Relations
	specRelations := []string{"in", "on", "under", "behind", "worn", "held", "seated", "contained"}
	engineRelations := []string{
		string(sim.RelIn),
		string(sim.RelOn),
		string(sim.RelUnder),
		string(sim.RelBehind),
		string(sim.RelWorn),
		string(sim.RelHeld),
		string(sim.RelSeated),
		string(sim.RelContained),
	}
	slices.Sort(specRelations)
	slices.Sort(engineRelations)
	if !slices.Equal(specRelations, engineRelations) {
		t.Fatalf("relation enum mismatch: spec=%v engine=%v", specRelations, engineRelations)
	}

	// 2. Subject Kinds
	specSubjectKinds := []string{"person", "place", "object", "group", "idea"}
	engineSubjectKinds := []string{
		string(memory.SubjectPerson),
		string(memory.SubjectPlace),
		string(memory.SubjectObject),
		string(memory.SubjectGroup),
		string(memory.SubjectIdea),
	}
	slices.Sort(specSubjectKinds)
	slices.Sort(engineSubjectKinds)
	if !slices.Equal(specSubjectKinds, engineSubjectKinds) {
		t.Fatalf("subject kind enum mismatch: spec=%v engine=%v", specSubjectKinds, engineSubjectKinds)
	}

	// 3. Claim Kinds
	specClaimKinds := []string{"belief", "feeling", "suspicion", "intention"}
	engineClaimKinds := []string{
		string(memory.ClaimBelief),
		string(memory.ClaimFeeling),
		string(memory.ClaimSuspicion),
		string(memory.ClaimIntention),
	}
	slices.Sort(specClaimKinds)
	slices.Sort(engineClaimKinds)
	if !slices.Equal(specClaimKinds, engineClaimKinds) {
		t.Fatalf("claim kind enum mismatch: spec=%v engine=%v", specClaimKinds, engineClaimKinds)
	}

	// 4. Binding Modes
	specBindingModes := []string{"identity", "anchor"}
	engineBindingModes := []string{
		string(memory.BindingIdentity),
		string(memory.BindingAnchor),
	}
	slices.Sort(specBindingModes)
	slices.Sort(engineBindingModes)
	if !slices.Equal(specBindingModes, engineBindingModes) {
		t.Fatalf("binding mode enum mismatch: spec=%v engine=%v", specBindingModes, engineBindingModes)
	}

	// 5. Attribute Visibility
	specVisibilities := []string{"public", "private"}
	engineVisibilities := []string{
		string(sim.VisibilityPublic),
		string(sim.VisibilityPrivate),
	}
	slices.Sort(specVisibilities)
	slices.Sort(engineVisibilities)
	if !slices.Equal(specVisibilities, engineVisibilities) {
		t.Fatalf("visibility enum mismatch: spec=%v engine=%v", specVisibilities, engineVisibilities)
	}

	// 6. Temporal Precision
	specPrecisions := []string{"minute", "hour", "day", "month", "year"}
	enginePrecisions := []string{
		string(memory.PrecisionMinute),
		string(memory.PrecisionHour),
		string(memory.PrecisionDay),
		string(memory.PrecisionMonth),
		string(memory.PrecisionYear),
	}
	slices.Sort(specPrecisions)
	slices.Sort(enginePrecisions)
	if !slices.Equal(specPrecisions, enginePrecisions) {
		t.Fatalf("precision enum mismatch: spec=%v engine=%v", specPrecisions, enginePrecisions)
	}
}

// TestGoldenSchemaFreshness ensures the committed schema file schema/omnia.scenario.v1.json
// matches the freshly generated schema from pkg/scenario/spec types.
func TestGoldenSchemaFreshness(t *testing.T) {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}

	schema := reflector.Reflect(&spec.Document{})
	schema.ID = "https://omnia.adityagupta.dev/schema/omnia.scenario.v1.json"
	schema.Title = "Omnia Scenario Specification v1.0"
	schema.Description = "JSON Schema contract for Omnia multi-agent narrative scenarios."

	freshData, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		t.Fatalf("failed marshaling fresh schema: %v", err)
	}
	freshData = append(freshData, '\n')

	committedData, err := os.ReadFile("../../../schema/omnia.scenario.v1.json")
	if err != nil {
		t.Fatalf("failed reading committed schema: %v", err)
	}

	if string(freshData) != string(committedData) {
		t.Fatalf("committed schema is stale. Run 'go run tools/schemagen/main.go' to regenerate")
	}
}
