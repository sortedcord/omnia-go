package actor

import (
	"strings"
	"testing"

	"omnia.adityagupta.dev/pkg/memory"
)

func TestActorAddThoughtPopulatesSelfDossierAndPrompt(t *testing.T) {
	mind := New(Profile{ID: "nelly", Name: "Nelly"})

	// Seed an intention with links (which used to be misfiled under the link entity!)
	mind.AddThought(Thought{
		ID:       "th-1",
		Kind:     ThoughtIntention,
		Content:  "Explain the housing audit without dying of embarrassment.",
		Links:    []string{"relationship-notes", "living-room"},
		Salience: 0.95,
	})

	// Seed a feeling with links
	mind.AddThought(Thought{
		ID:       "th-2",
		Kind:     ThoughtFeeling,
		Content:  "Cheeks are burning like hot radiators.",
		Links:    []string{"living-room"},
		Salience: 0.85,
	})

	// 1. Verify claims were filed in Nelly's self-dossier (subject ID "nelly")
	selfDoss, ok := mind.Mind.GetDossier("nelly")
	if !ok {
		t.Fatal("expected self-dossier for nelly to exist")
	}
	if len(selfDoss.Claims) != 2 {
		t.Fatalf("expected 2 claims in self-dossier, got %d", len(selfDoss.Claims))
	}

	// 2. Render prompt context: selfText must contain both claims
	cues := memory.RecallCues{CurrentTick: 1}
	selfText, _, _ := mind.Mind.RenderContext(cues)

	if !strings.Contains(selfText, "Explain the housing audit") {
		t.Errorf("expected intention in selfText:\n%s", selfText)
	}
	if !strings.Contains(selfText, "Cheeks are burning") {
		t.Errorf("expected feeling in selfText:\n%s", selfText)
	}

	// 3. Verify buildMessages includes the self-dossier in developer messages
	stim := SensoryEvent{
		Tick:        1,
		Timestamp:   "18:35:00 UTC",
		Description: "Zack arrived.",
		SelfContext: selfText,
	}
	msgs := mind.buildMessages(stim)

	found := false
	for _, m := range msgs {
		if strings.Contains(m.Content, "YOUR CURRENT STATE OF MIND") && strings.Contains(m.Content, "Explain the housing audit") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected self-dossier with intention in buildMessages developer context")
	}
}
