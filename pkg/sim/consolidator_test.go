package sim

import (
	"context"
	"testing"
	"time"

	"omnia.adityagupta.dev/pkg/llm"
	"omnia.adityagupta.dev/pkg/memory"
)

func TestConsolidatorSchemaValidation(t *testing.T) {
	s1 := ConsolidatorSchema(nil)
	if err := llm.ValidateStrict(s1); err != nil {
		t.Fatalf("expected ConsolidatorSchema(nil) to pass ValidateStrict: %v", err)
	}

	s2 := ConsolidatorSchema([]string{"s-zack", "s-nelly"})
	if err := llm.ValidateStrict(s2); err != nil {
		t.Fatalf("expected ConsolidatorSchema with subjects to pass ValidateStrict: %v", err)
	}
}

type recordingConsolidatorClient struct {
	lastReq llm.ChatRequest
	resp    *llm.ChatResponse
}

func (c *recordingConsolidatorClient) ChatCompletion(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	c.lastReq = req
	if c.resp != nil {
		return c.resp, nil
	}
	return &llm.ChatResponse{
		Content: `{
			"beats": [
				{
					"text": "Zack arrived at the apartment and noticed my color-coded notes.",
					"subject_ids": ["s-zack", "s-notes"]
				},
				{
					"text": "I frantically explained the housing inspection while Mia hid in the kitchen.",
					"subject_ids": ["s-zack", "s-mia"]
				}
			],
			"title": "Zack arrives and reviews notes",
			"summary": "Zack came up to help with the housing inspection. I completely spiraled over the notes, but he stayed dry and calm.",
			"tone": "mortified relief",
			"importance": 0.85,
			"formative_subject_ids": ["s-zack"]
		}`,
		Duration: 15 * time.Millisecond,
	}, nil
}

func TestConsolidatorExecution(t *testing.T) {
	client := &recordingConsolidatorClient{}
	consolidator := NewLLMConsolidator(client, "test-model")

	input := ConsolidateInput{
		ActorID:     "nelly",
		ActorName:   "Nelly",
		Personality: "Warm and flustered",
		SceneID:     "scene-1",
		LocationID:  "living-room",
		FromSeq:     1,
		ToSeq:       2,
		Turns: []memory.Turn{
			{
				Seq:        1,
				Tick:       1,
				Perception: "Zack enters the apartment.",
				Action:     memory.ActionRecord{Type: "speak", Action: "I made notes."},
			},
			{
				Seq:        2,
				Tick:       1,
				Perception: "Zack smiles dryly.",
				Action:     memory.ActionRecord{Type: "speak", Action: "Please don't think I'm crazy."},
			},
		},
		IsSceneClose:        true,
		AvailableSubjectIDs: []string{"s-zack", "s-notes", "s-mia"},
	}

	res, err := consolidator.Consolidate(context.Background(), input)
	if err != nil {
		t.Fatalf("Consolidate failed: %v", err)
	}

	// 1. Verify schema was used
	if client.lastReq.ResponseFormat == nil || client.lastReq.ResponseFormat.Type != "json_schema" {
		t.Fatalf("expected json_schema ResponseFormat, got: %+v", client.lastReq.ResponseFormat)
	}
	if client.lastReq.ResponseFormat.JSONSchema.Name != "episode_consolidation" {
		t.Fatalf("unexpected schema name: %s", client.lastReq.ResponseFormat.JSONSchema.Name)
	}

	// 2. Verify results
	if len(res.Beats) != 2 {
		t.Fatalf("expected 2 beats, got %d", len(res.Beats))
	}
	if res.Title != "Zack arrives and reviews notes" {
		t.Errorf("unexpected title: %q", res.Title)
	}
	if res.Tone != "mortified relief" {
		t.Errorf("unexpected tone: %q", res.Tone)
	}
	if res.Importance != 0.85 {
		t.Errorf("unexpected importance: %v", res.Importance)
	}
	if len(res.FormativeSubjectIDs) != 1 || res.FormativeSubjectIDs[0] != "s-zack" {
		t.Errorf("unexpected formative subjects: %v", res.FormativeSubjectIDs)
	}
}
