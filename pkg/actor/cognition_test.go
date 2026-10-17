package actor

import (
	"strings"
	"testing"
)

func TestCleanCognitionPreservesAuthenticThoughts(t *testing.T) {
	raw := "Oh god, Zack is looking right at me. My heart is pounding and my sleeves are soaked with sweat. I need to explain the housing board without passing out."
	cleaned := CleanCognition(raw)
	if cleaned != raw {
		t.Fatalf("expected untouched thoughts, got:\n%s", cleaned)
	}
}

func TestCleanCognitionStripsGatewayChecklistBoilerplate(t *testing.T) {
	raw := `Thinking Process:

1. Analyze the Request:
   - Zack commented on the notes.
   - Nelly must respond in character.

2. Formulate Reaction:
   - *Oh god, why did I say notes? My cheeks are boiling.*

3. Validate against constraints:
   - Valid, parseable JSON? Yes.
   - Scalars, boolean, null, other values double-quoted? Yes.
   - Do not include any extra text outside of the JSON string.
   - No control tokens like \n or \t.
   - Final JSON string formulation: done.`

	cleaned := CleanCognition(raw)

	if strings.Contains(cleaned, "Valid, parseable JSON") {
		t.Errorf("cleaned cognition still contains 'Valid, parseable JSON':\n%s", cleaned)
	}
	if strings.Contains(cleaned, "Scalars, boolean, null") {
		t.Errorf("cleaned cognition still contains 'Scalars, boolean, null':\n%s", cleaned)
	}
	if strings.Contains(cleaned, "Do not include any extra text") {
		t.Errorf("cleaned cognition still contains 'Do not include extra text':\n%s", cleaned)
	}
	if !strings.Contains(cleaned, "My cheeks are boiling") {
		t.Errorf("cleaned cognition lost authentic thoughts:\n%s", cleaned)
	}
}
