package sim

import (
	"context"
	"strings"
	"testing"
	"time"

	"omnia.adityagupta.dev/pkg/llm"
)

func TestDistillerSchemaValidation(t *testing.T) {
	s1 := DistillerSchema(nil)
	if err := llm.ValidateStrict(s1); err != nil {
		t.Fatalf("expected DistillerSchema(nil) to pass ValidateStrict: %v", err)
	}

	s2 := DistillerSchema([]string{"entity-1", "entity-2"})
	if err := llm.ValidateStrict(s2); err != nil {
		t.Fatalf("expected DistillerSchema with links to pass ValidateStrict: %v", err)
	}
}

type recordingDistillerClient struct {
	lastReq llm.ChatRequest
	resp    *llm.ChatResponse
}

func (c *recordingDistillerClient) ChatCompletion(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	c.lastReq = req
	if c.resp != nil {
		return c.resp, nil
	}
	return &llm.ChatResponse{
		Content: `{
			"ops": [
				{"op": "add", "id": null, "kind": "belief", "content": "The room is quiet.", "links": [], "salience": 1.50},
				{"op": "update", "id": "th-1", "kind": null, "content": "Updated plan.", "links": ["entity-1"], "salience": 0.01},
				{"op": "drop", "id": "th-2", "kind": null, "content": null, "links": [], "salience": null},
				{"op": "add", "id": null, "kind": "feeling", "content": "Nervous.", "links": [], "salience": 0.50},
				{"op": "add", "id": null, "kind": "suspicion", "content": "Excess op 5 should be capped.", "links": [], "salience": 0.50}
			]
		}`,
		Duration: 10 * time.Millisecond,
	}, nil
}

func TestDistillerStructuredOutputsAndClamping(t *testing.T) {
	client := &recordingDistillerClient{}
	distiller := NewLLMDistiller(client, "test-model")

	input := DistillInput{
		ActorID:   "zack",
		ActorName: "Zack",
		Tick:      1,
		Perceivable: []EntityRef{
			{ID: "entity-1", Name: "Table"},
		},
		KnownEntities: map[string]string{
			"kitchenette": "Open Kitchenette",
		},
	}

	res, err := distiller.Distill(context.Background(), input)
	if err != nil {
		t.Fatalf("Distill failed: %v", err)
	}

	// 1. Verify schema was attached
	req := client.lastReq
	if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_schema" {
		t.Fatalf("expected json_schema ResponseFormat, got: %+v", req.ResponseFormat)
	}
	if req.ResponseFormat.JSONSchema.Name != "thought_distillation" {
		t.Fatalf("unexpected schema name: %s", req.ResponseFormat.JSONSchema.Name)
	}

	// 2. Verify system prompt does not contain schema skeleton or OUTPUT REQUIREMENT
	sysMsg := req.Messages[0].Content
	if strings.Contains(strings.ToLower(sysMsg), "output requirement") {
		t.Errorf("system prompt contains 'OUTPUT REQUIREMENT':\n%s", sysMsg)
	}
	if strings.Contains(sysMsg, `"ops":`) || strings.Contains(sysMsg, `"salience":`) {
		t.Errorf("system prompt contains JSON schema skeleton:\n%s", sysMsg)
	}

	// 3. Verify ops were capped at 4
	if len(res.Ops) != 4 {
		t.Fatalf("expected exactly 4 ops (capped from 5), got %d", len(res.Ops))
	}

	// 4. Verify salience was clamped
	// op 0 was 1.50 -> clamped to 1.00
	if res.Ops[0].Salience == nil || *res.Ops[0].Salience != 1.00 {
		t.Errorf("expected op 0 salience clamped to 1.00, got: %v", res.Ops[0].Salience)
	}
	// op 1 was 0.01 -> clamped to 0.05
	if res.Ops[1].Salience == nil || *res.Ops[1].Salience != 0.05 {
		t.Errorf("expected op 1 salience clamped to 0.05, got: %v", res.Ops[1].Salience)
	}
	// op 2 had null salience -> stays nil
	if res.Ops[2].Salience != nil {
		t.Errorf("expected op 2 salience to be nil, got: %v", *res.Ops[2].Salience)
	}
}
