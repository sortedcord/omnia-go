package llm

import (
	"encoding/json"
	"strings"
)

type wireResponseFormat struct {
	Type string `json:"type"`
}

type wireRequest struct {
	Model            string              `json:"model"`
	Messages         []Message           `json:"messages"`
	ResponseFormat   *wireResponseFormat `json:"response_format,omitempty"`
	ReasoningEffort  string              `json:"reasoning_effort,omitempty"`
	IncludeReasoning *bool               `json:"include_reasoning,omitempty"`
}

type wireReasoningDetail struct {
	Index     int    `json:"index"`
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Signature string `json:"signature,omitempty"`
}

type wireResponseMessage struct {
	Role             Role                  `json:"role"`
	Content          string                `json:"content"`
	ReasoningContent string                `json:"reasoning_content,omitempty"`
	Reasoning        string                `json:"reasoning,omitempty"`
	Thought          string                `json:"thought,omitempty"`
	Thinking         string                `json:"thinking,omitempty"`
	ReasoningDetails []wireReasoningDetail `json:"reasoning_details,omitempty"`
	RawFields        map[string]any        `json:"-"`
}

func (m *wireResponseMessage) UnmarshalJSON(data []byte) error {
	type Alias wireResponseMessage
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err == nil {
		m.RawFields = raw
	}
	return nil
}

type wireChoice struct {
	Index            int                 `json:"index"`
	Message          wireResponseMessage `json:"message"`
	ReasoningContent string              `json:"reasoning_content,omitempty"`
	Reasoning        string              `json:"reasoning,omitempty"`
	FinishReason     string              `json:"finish_reason"`
}

type wireUsage struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	TotalTokens             int `json:"total_tokens"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

type wireResponse struct {
	ID      string       `json:"id"`
	Choices []wireChoice `json:"choices"`
	Usage   wireUsage    `json:"usage"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

// extractThinking gathers thinking tokens across multiple provider formats
// (reasoning_details, reasoning_content, thought, or inline <think> tags).
func extractThinking(choice wireChoice) (string, string) {
	// 1. Structured reasoning_details array (e.g. Antigravity / Gemini proxies)
	var reasoningTexts []string
	for _, detail := range choice.Message.ReasoningDetails {
		if (detail.Type == "reasoning.text" || detail.Type == "") && strings.TrimSpace(detail.Text) != "" {
			reasoningTexts = append(reasoningTexts, strings.TrimSpace(detail.Text))
		}
	}
	if len(reasoningTexts) > 0 {
		return strings.Join(reasoningTexts, "\n\n"), choice.Message.Content
	}

	// 2. Direct fields on Message
	if strings.TrimSpace(choice.Message.ReasoningContent) != "" {
		return strings.TrimSpace(choice.Message.ReasoningContent), choice.Message.Content
	}
	if strings.TrimSpace(choice.Message.Reasoning) != "" {
		return strings.TrimSpace(choice.Message.Reasoning), choice.Message.Content
	}
	if strings.TrimSpace(choice.Message.Thought) != "" {
		return strings.TrimSpace(choice.Message.Thought), choice.Message.Content
	}
	if strings.TrimSpace(choice.Message.Thinking) != "" {
		return strings.TrimSpace(choice.Message.Thinking), choice.Message.Content
	}

	// 3. Direct fields on Choice
	if strings.TrimSpace(choice.ReasoningContent) != "" {
		return strings.TrimSpace(choice.ReasoningContent), choice.Message.Content
	}
	if strings.TrimSpace(choice.Reasoning) != "" {
		return strings.TrimSpace(choice.Reasoning), choice.Message.Content
	}

	// 4. Embedded in Content like <think>...</think> or <thought>...</thought>
	content := choice.Message.Content
	for _, tag := range []string{"think", "thought", "reasoning"} {
		startTag := "<" + tag + ">"
		endTag := "</" + tag + ">"
		start := strings.Index(content, startTag)
		end := strings.Index(content, endTag)
		if start != -1 && end != -1 && end > start {
			thinking := strings.TrimSpace(content[start+len(startTag) : end])
			clean := strings.TrimSpace(content[:start] + content[end+len(endTag):])
			return thinking, clean
		}
	}

	// 5. Dynamic fallback on any unrecognized field matching thought/reason/think
	for k, v := range choice.Message.RawFields {
		lower := strings.ToLower(k)
		if strings.Contains(lower, "reason") || strings.Contains(lower, "thought") || strings.Contains(lower, "think") {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s), choice.Message.Content
			}
		}
	}

	return "", choice.Message.Content
}
