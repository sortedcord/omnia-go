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

// ChatRequest is the decoupled domain request for a chat completion.
type ChatRequest struct {
	Model            string
	Messages         []Message
	ResponseFormat   string // e.g. "json_object"
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
