package sim

import (
	"context"
	"strings"
	"testing"
	"time"

	"omnia.adityagupta.dev/pkg/actor"
	"omnia.adityagupta.dev/pkg/llm"
)

func TestBroadcasterSchemaValidation(t *testing.T) {
	s1 := BroadcasterSchema([]EntityID{"zack", "nelly"})
	if err := llm.ValidateStrict(s1); err != nil {
		t.Fatalf("expected BroadcasterSchema with actors to pass ValidateStrict: %v", err)
	}

	s2 := BroadcasterSchema(nil)
	if err := llm.ValidateStrict(s2); err != nil {
		t.Fatalf("expected BroadcasterSchema(nil) to pass ValidateStrict: %v", err)
	}
}

func TestArbiterSchemaValidation(t *testing.T) {
	s1 := ArbiterSchema([]EntityID{"zack", "nelly"}, []string{"chair", "door", "table"})
	if err := llm.ValidateStrict(s1); err != nil {
		t.Fatalf("expected ArbiterSchema with entities to pass ValidateStrict: %v", err)
	}

	s2 := ArbiterSchema(nil, nil)
	if err := llm.ValidateStrict(s2); err != nil {
		t.Fatalf("expected ArbiterSchema(nil, nil) to pass ValidateStrict: %v", err)
	}
}

type recordingArbiterClient struct {
	lastReq llm.ChatRequest
	resp    *llm.ChatResponse
}

func (c *recordingArbiterClient) ChatCompletion(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	c.lastReq = req
	if c.resp != nil {
		return c.resp, nil
	}
	return &llm.ChatResponse{
		Content: `{
			"execution_order": ["zack", "nelly"],
			"verdicts": {
				"zack": {"verdict": "as_intended", "outcome": "success", "remaining_intent": null},
				"nelly": {"verdict": "altered", "outcome": "partial", "remaining_intent": "explain housing details"}
			},
			"resolution": "Zack reassured Nelly, while Nelly stumbled through her notes.",
			"effects": [
				{
					"type": "set_attribute",
					"entity_id": "nelly",
					"parent_id": null,
					"relation": null,
					"placement": null,
					"key": "mood",
					"value": "relieved",
					"visibility": "public",
					"accessors": null,
					"closed": null,
					"locked": null,
					"open": null
				},
				{
					"type": "move",
					"entity_id": "chair",
					"parent_id": "table",
					"relation": "under",
					"placement": "tucked under",
					"key": null,
					"value": null,
					"visibility": null,
					"accessors": null,
					"closed": null,
					"locked": null,
					"open": null
				}
			]
		}`,
		Duration: 15 * time.Millisecond,
	}, nil
}

func TestArbiterStructuredOutputsAdjudication(t *testing.T) {
	client := &recordingArbiterClient{}
	arbiter := NewLLMArbiter(client, "test-model")

	adj := SceneAdjudication{
		SceneID: "scene-1",
		Tick:    1,
		Intents: []ActorIntent{
			{ActorID: "zack", ActorName: "Zack", ActionType: actor.Speak, ActionText: "Notes are good."},
			{ActorID: "nelly", ActorName: "Nelly", ActionType: actor.Speak, ActionText: "I made notes."},
		},
		WorldState: "SPACE: living-room [ID: living-room]\n- chair [ID: chair]\n- table [ID: table]",
	}

	ruling, err := arbiter.ResolveScene(context.Background(), adj)
	if err != nil {
		t.Fatalf("ResolveScene failed: %v", err)
	}

	// 1. Verify schema was attached
	req := client.lastReq
	if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_schema" {
		t.Fatalf("expected json_schema ResponseFormat, got: %+v", req.ResponseFormat)
	}
	if req.ResponseFormat.JSONSchema.Name != "scene_ruling" {
		t.Fatalf("unexpected schema name: %s", req.ResponseFormat.JSONSchema.Name)
	}

	// 2. Verify prompt has no schema boilerplate or OUTPUT REQUIREMENT
	sysMsg := req.Messages[0].Content
	if strings.Contains(strings.ToLower(sysMsg), "output requirement") {
		t.Errorf("system prompt contains 'OUTPUT REQUIREMENT':\n%s", sysMsg)
	}
	if strings.Contains(sysMsg, `"execution_order":`) || strings.Contains(sysMsg, `"verdicts":`) {
		t.Errorf("system prompt contains JSON schema skeleton:\n%s", sysMsg)
	}

	// 3. Verify ruling fields
	if len(ruling.ExecutionOrder) != 2 || ruling.ExecutionOrder[0] != "zack" || ruling.ExecutionOrder[1] != "nelly" {
		t.Fatalf("unexpected execution order: %v", ruling.ExecutionOrder)
	}

	if len(ruling.Verdicts) != 2 {
		t.Fatalf("expected 2 verdicts, got %d", len(ruling.Verdicts))
	}
	if ruling.Verdicts[0].ActorID != "zack" || ruling.Verdicts[0].Verdict != "as_intended" || ruling.Verdicts[0].Outcome != "success" {
		t.Errorf("unexpected zack verdict: %+v", ruling.Verdicts[0])
	}
	if ruling.Verdicts[1].ActorID != "nelly" || ruling.Verdicts[1].Verdict != "altered" || ruling.Verdicts[1].RemainingIntent != "explain housing details" {
		t.Errorf("unexpected nelly verdict: %+v", ruling.Verdicts[1])
	}

	// 4. Verify effects
	if len(ruling.Effects) != 2 {
		t.Fatalf("expected 2 effects, got %d", len(ruling.Effects))
	}
	if ruling.Effects[0].Type != EffectSetAttribute || ruling.Effects[0].Key != "mood" || ruling.Effects[0].Value != "relieved" {
		t.Errorf("unexpected effect 0: %+v", ruling.Effects[0])
	}
	if ruling.Effects[1].Type != EffectMove || ruling.Effects[1].EntityID != "chair" || ruling.Effects[1].ParentID != "table" || ruling.Effects[1].Relation != RelUnder {
		t.Errorf("unexpected effect 1: %+v", ruling.Effects[1])
	}
}

func TestBroadcasterStructuredOutputs(t *testing.T) {
	var capturedReq llm.ChatRequest
	fakeClient := &recordingBroadcasterClient{
		onCall: func(req llm.ChatRequest) {
			capturedReq = req
		},
	}
	broadcaster := NewLLMBroadcaster(fakeClient, "test-model")

	in := BroadcastInput{
		SceneID:    "scene-1",
		Tick:       1,
		Resolution: "Zack and Nelly talked.",
		Actors: []PerceiverInfo{
			{ID: "zack", Name: "Zack"},
			{ID: "nelly", Name: "Nelly"},
		},
	}

	res, err := broadcaster.Broadcast(context.Background(), in)
	if err != nil {
		t.Fatalf("Broadcast failed: %v", err)
	}

	if capturedReq.ResponseFormat == nil || capturedReq.ResponseFormat.Type != "json_schema" {
		t.Fatalf("expected json_schema format on broadcaster, got: %+v", capturedReq.ResponseFormat)
	}
	if capturedReq.ResponseFormat.JSONSchema.Name != "scene_perceptions" {
		t.Fatalf("unexpected schema name: %s", capturedReq.ResponseFormat.JSONSchema.Name)
	}

	if res.Perceptions["zack"] != "You see Nelly." || res.Perceptions["nelly"] != "You see Zack." {
		t.Fatalf("unexpected perceptions: %+v", res.Perceptions)
	}
}

type recordingBroadcasterClient struct {
	onCall func(req llm.ChatRequest)
}

func (c *recordingBroadcasterClient) ChatCompletion(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if c.onCall != nil {
		c.onCall(req)
	}
	return &llm.ChatResponse{
		Content: `{
			"perceptions": {
				"zack": "You see Nelly.",
				"nelly": "You see Zack."
			}
		}`,
		Duration: 10 * time.Millisecond,
	}, nil
}
