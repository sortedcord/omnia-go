package actor

import (
	"context"
	"strings"
	"testing"
	"time"

	"omnia.adityagupta.dev/pkg/llm"
)

func TestActorActionSchemaValidation(t *testing.T) {
	// Verify ActionSchema conforms to strict Structured Outputs rules
	s1 := ActionSchema(nil)
	if err := llm.ValidateStrict(s1); err != nil {
		t.Fatalf("expected ActionSchema(nil) to pass ValidateStrict, got: %v", err)
	}

	s2 := ActionSchema([]string{"Zack", "Mia", "Nelly"})
	if err := llm.ValidateStrict(s2); err != nil {
		t.Fatalf("expected ActionSchema with targets to pass ValidateStrict, got: %v", err)
	}
}

func TestActorSystemPromptsHaveNoSchemaBoilerplate(t *testing.T) {
	prompts := DefaultSystemPrompts("Nelly")
	for _, p := range prompts {
		lower := strings.ToLower(p.Content)
		if strings.Contains(lower, "output requirement") {
			t.Errorf("prompt %q contains 'OUTPUT REQUIREMENT'", p.Name)
		}
		if strings.Contains(lower, "action_type") {
			t.Errorf("prompt %q contains 'action_type'", p.Name)
		}
		if strings.Contains(p.Content, `"action":`) || strings.Contains(p.Content, `"targets":`) {
			t.Errorf("prompt %q contains JSON schema skeleton", p.Name)
		}
	}
}

type recordingLLMClient struct {
	lastReq llm.ChatRequest
	resp    *llm.ChatResponse
	err     error
}

func (m *recordingLLMClient) ChatCompletion(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	m.lastReq = req
	if m.err != nil {
		return nil, m.err
	}
	if m.resp != nil {
		return m.resp, nil
	}
	return &llm.ChatResponse{
		Content:  `{"action_type":"speak","action":"Hello","targets":["Zack"]}`,
		Duration: 10 * time.Millisecond,
	}, nil
}

func TestActorReactSendsStrictSchema(t *testing.T) {
	fakeClient := &recordingLLMClient{}
	actor := New(Profile{ID: "nelly", Name: "Nelly"}, WithClient(fakeClient))

	stimulus := SensoryEvent{
		Tick:        1,
		Timestamp:   "18:35:00 UTC",
		Description: "Zack arrived.",
	}

	rx, err := actor.React(context.Background(), stimulus)
	if err != nil {
		t.Fatalf("React failed: %v", err)
	}

	if rx.Action.Type != Speak || rx.Action.Action != "Hello" {
		t.Fatalf("unexpected action: %+v", rx.Action)
	}

	// Verify that the request carried the strict JSON schema
	req := fakeClient.lastReq
	if req.ResponseFormat == nil {
		t.Fatal("expected ResponseFormat to be non-nil")
	}
	if req.ResponseFormat.Type != "json_schema" {
		t.Fatalf("expected ResponseFormat.Type to be 'json_schema', got %q", req.ResponseFormat.Type)
	}
	if req.ResponseFormat.JSONSchema == nil {
		t.Fatal("expected JSONSchema to be non-nil")
	}
	if req.ResponseFormat.JSONSchema.Name != "actor_action" {
		t.Fatalf("expected schema name 'actor_action', got %q", req.ResponseFormat.JSONSchema.Name)
	}
	if !req.ResponseFormat.JSONSchema.Strict {
		t.Fatal("expected Strict: true on schema")
	}
}
