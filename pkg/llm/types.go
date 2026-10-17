package llm

import (
	"encoding/json"
	"fmt"
	"time"
)

// Role represents a message author role in chat completions.
type Role int

const (
	System Role = iota
	Developer
	Assistant
	User
)

func (r Role) String() string {
	switch r {
	case System:
		return "system"
	case Developer:
		return "developer"
	case Assistant:
		return "assistant"
	case User:
		return "user"
	default:
		return fmt.Sprintf("Role(%d)", r)
	}
}

func (r Role) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

func (r *Role) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "system":
		*r = System
	case "developer":
		*r = Developer
	case "assistant":
		*r = Assistant
	case "user":
		*r = User
	default:
		return fmt.Errorf("unknown role: %q", s)
	}
	return nil
}

// Message represents a single chat turn.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// ReasoningEffort sets the reasoning deliberation depth for reasoning models.
type ReasoningEffort string

const (
	ReasoningLow    ReasoningEffort = "low"
	ReasoningMedium ReasoningEffort = "medium"
	ReasoningHigh   ReasoningEffort = "high"
)

// ResponseFormat defines the structural constraint on model output.
type ResponseFormat struct {
	Type       string      `json:"type"` // "json_object" or "json_schema"
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

// JSONSchema holds the specification for OpenAI-compatible Structured Outputs.
type JSONSchema struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Strict      bool    `json:"strict"`
	Schema      *Schema `json:"schema"`
}

// JSONObject returns a response format requesting generic valid JSON object output.
func JSONObject() *ResponseFormat {
	return &ResponseFormat{Type: "json_object"}
}

// StrictSchema returns a response format enforcing strict Structured Outputs schema adherence.
func StrictSchema(name, description string, s *Schema) *ResponseFormat {
	return &ResponseFormat{
		Type: "json_schema",
		JSONSchema: &JSONSchema{
			Name:        name,
			Description: description,
			Strict:      true,
			Schema:      s,
		},
	}
}

// ChatRequest is the decoupled domain request for a chat completion.
type ChatRequest struct {
	Model            string
	Messages         []Message
	ResponseFormat   *ResponseFormat
	ReasoningEffort  ReasoningEffort
	IncludeReasoning bool
}

// Usage holds token consumption statistics.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	ReasoningTokens  int
	TotalTokens      int
}

// ChatResponse contains the clean, extracted completion results.
type ChatResponse struct {
	Content          string // Final manifest content (stripped of inline thinking tags if any)
	ReasoningContent string // Private internal thinking/reasoning stream
	Usage            Usage
	Duration         time.Duration // Time taken for the request round-trip
}
