package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSchemaMarshalOrderingAndStrictInvariants(t *testing.T) {
	// Construct an object with intentionally non-alphabetical property order:
	// "zeta", "alpha", "mid"
	schema := Obj("test object",
		Prop("zeta", Str("the zeta property")),
		Prop("alpha", Nullable(Enum("the alpha enum", "val1", "val2"))),
		Prop("mid", Arr("the mid array", Obj("nested item",
			Prop("sub_b", Str("sub b")),
			Prop("sub_a", Num("sub a")),
		))),
	)

	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	raw := string(data)

	// 1. In properties, "zeta" must appear before "alpha", and "alpha" before "mid"
	idxZeta := strings.Index(raw, `"zeta"`)
	idxAlpha := strings.Index(raw, `"alpha"`)
	idxMid := strings.Index(raw, `"mid"`)

	if idxZeta == -1 || idxAlpha == -1 || idxMid == -1 {
		t.Fatalf("missing properties in raw JSON: %s", raw)
	}
	if !(idxZeta < idxAlpha && idxAlpha < idxMid) {
		t.Fatalf("expected property order zeta < alpha < mid, got indices: zeta=%d, alpha=%d, mid=%d in %s",
			idxZeta, idxAlpha, idxMid, raw)
	}

	// 2. In required array, ["zeta","alpha","mid"] must appear in that exact order
	expectedReq := `"required":["zeta","alpha","mid"]`
	if !strings.Contains(raw, expectedReq) {
		t.Fatalf("expected required array %q in %s", expectedReq, raw)
	}

	// 3. additionalProperties: false must be present
	if !strings.Contains(raw, `"additionalProperties":false`) {
		t.Fatalf("expected additionalProperties:false in %s", raw)
	}

	// 4. Nullable enum must have ["string","null"] and null in enum
	if !strings.Contains(raw, `"type":["string","null"]`) {
		t.Fatalf("expected nullable type in %s", raw)
	}
	if !strings.Contains(raw, `"enum":["val1","val2",null]`) {
		t.Fatalf("expected null in enum in %s", raw)
	}

	// 5. Nested object also has ordered properties and additionalProperties: false
	idxSubB := strings.Index(raw, `"sub_b"`)
	idxSubA := strings.Index(raw, `"sub_a"`)
	if !(idxSubB < idxSubA) {
		t.Fatalf("expected nested property order sub_b < sub_a in %s", raw)
	}
	expectedNestedReq := `"required":["sub_b","sub_a"]`
	if !strings.Contains(raw, expectedNestedReq) {
		t.Fatalf("expected nested required array %q in %s", expectedNestedReq, raw)
	}
}

func TestValidateStrict(t *testing.T) {
	// Valid schemas
	valid1 := Obj("simple", Prop("name", Str("person name")))
	if err := ValidateStrict(valid1); err != nil {
		t.Errorf("expected valid1 to pass, got: %v", err)
	}

	valid2 := Obj("with array and enum",
		Prop("tags", Arr("list of tags", Enum("allowed tags", "t1", "t2"))),
		Prop("count", Num("count")),
		Prop("active", Bool("active flag")),
	)
	if err := ValidateStrict(valid2); err != nil {
		t.Errorf("expected valid2 to pass, got: %v", err)
	}

	// Invalid: nil schema
	if err := ValidateStrict(nil); err == nil {
		t.Error("expected error for nil schema")
	}

	// Invalid: empty object (no properties)
	emptyObj := &Schema{Type: []string{"object"}}
	if err := ValidateStrict(emptyObj); err == nil {
		t.Error("expected error for object with no properties")
	}

	// Invalid: duplicate property name
	dupProp := Obj("dup", Prop("k", Str("")), Prop("k", Num("")))
	if err := ValidateStrict(dupProp); err == nil {
		t.Error("expected error for duplicate property name")
	}

	// Invalid: array without items
	arrNoItems := &Schema{Type: []string{"array"}}
	if err := ValidateStrict(arrNoItems); err == nil {
		t.Error("expected error for array without items")
	}

	// Invalid: nesting > 5
	deep := Str("leaf")
	for i := 0; i < 6; i++ {
		deep = Obj("level", Prop("child", deep))
	}
	if err := ValidateStrict(deep); err == nil {
		t.Error("expected error for schema depth > 5")
	}

	// Invalid: unsupported type
	badType := &Schema{Type: []string{"unsupported_symbol"}}
	if err := ValidateStrict(badType); err == nil {
		t.Error("expected error for unsupported type")
	}
}

func TestPromptText(t *testing.T) {
	schema := Obj("action schema",
		Prop("action_type", Enum("speak or act", "speak", "act", "wait")),
		Prop("action", Str("action description")),
		Prop("targets", Arr("addressees", Str("actor name"))),
	)

	text := schema.PromptText()
	if !strings.Contains(text, "action_type") || !strings.Contains(text, "speak or act") {
		t.Errorf("expected action_type in prompt text, got:\n%s", text)
	}
	if !strings.Contains(text, `"speak" | "act" | "wait"`) {
		t.Errorf("expected enum values in prompt text, got:\n%s", text)
	}
}

func TestHTTPClientSchemaFallback(t *testing.T) {
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		body, _ := io.ReadAll(r.Body)

		var req map[string]any
		_ = json.Unmarshal(body, &req)

		rf, _ := req["response_format"].(map[string]any)
		rfType, _ := rf["type"].(string)

		if rfType == "json_schema" {
			// Simulate gateway rejecting json_schema with HTTP 400
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"type":"INVALID_ARGUMENT","message":"Google upstream request failed"}}`))
			return
		}

		if rfType == "json_object" {
			// Fallback succeeded! Verify prompt text was appended as a system message
			msgs, _ := req["messages"].([]any)
			lastMsg := msgs[len(msgs)-1].(map[string]any)
			content, _ := lastMsg["content"].(string)
			if !strings.Contains(content, "EXPECTED JSON OUTPUT SCHEMA:") {
				t.Errorf("expected prompt schema in fallback request, got: %s", content)
			}

			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"id": "test-123",
				"choices": [{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "{\"action_type\":\"speak\",\"action\":\"hello\"}"
					}
				}],
				"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
			}`))
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "dummy-key")

	schema := Obj("action",
		Prop("action_type", Enum("type", "speak", "act")),
		Prop("action", Str("desc")),
	)
	req := ChatRequest{
		Model:          "test-model",
		Messages:       []Message{{Role: User, Content: "Hello"}},
		ResponseFormat: StrictSchema("action_schema", "action", schema),
	}

	// 1. First call: attempts json_schema (fails 400), retries with json_object (succeeds 200)
	resp, err := client.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if resp.Content != `{"action_type":"speak","action":"hello"}` {
		t.Fatalf("unexpected content: %s", resp.Content)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts on first call (initial + fallback), got %d", attempts)
	}

	// 2. Second call: capability fallback should be cached; directly sends json_object (1 attempt)
	attempts = 0
	resp2, err := client.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("expected cached fallback to succeed, got error: %v", err)
	}
	if resp2.Content != `{"action_type":"speak","action":"hello"}` {
		t.Fatalf("unexpected content: %s", resp2.Content)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt on second call (using cached capability), got %d", attempts)
	}
}
