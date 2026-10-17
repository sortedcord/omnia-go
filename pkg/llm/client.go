package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client defines the contract for interacting with chat completion backends.
type Client interface {
	ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

// Option configures an HTTPClient.
type Option func(*HTTPClient)

// WithHTTPClient overrides the default http.Client.
func WithHTTPClient(c *http.Client) Option {
	return func(client *HTTPClient) {
		if c != nil {
			client.httpClient = c
		}
	}
}

// WithTimeout sets a custom default timeout on the underlying HTTP client.
func WithTimeout(d time.Duration) Option {
	return func(client *HTTPClient) {
		client.httpClient.Timeout = d
	}
}

// DefaultClient returns a Client initialized from environment variables
// (OMNIA_API_URL, OMNIA_API_KEY, OPENAI_BASE_URL, OPENAI_API_KEY).
func DefaultClient() Client {
	baseURL := os.Getenv("OMNIA_API_URL")
	if baseURL == "" {
		baseURL = os.Getenv("OPENAI_BASE_URL")
	}
	if baseURL == "" {
		baseURL = "https://ai.adityagupta.dev/v1"
	}

	apiKey := os.Getenv("OMNIA_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	return NewHTTPClient(baseURL, apiKey)
}

// HTTPClient implements Client using standard net/http against OpenAI-compatible endpoints.
type HTTPClient struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
}

// NewHTTPClient creates a new OpenAI-compatible Chat Completions client.
// baseURL can be the root (e.g. "https://api.example.com/v1") or full endpoint.
func NewHTTPClient(baseURL, apiKey string, opts ...Option) *HTTPClient {
	endpoint := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}

	client := &HTTPClient{
		endpoint: endpoint,
		apiKey:   apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// ChatCompletion sends a request to the chat completions API and returns the parsed result.
func (c *HTTPClient) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	wireReq := wireRequest{
		Model:            req.Model,
		Messages:         req.Messages,
		ReasoningEffort:  string(req.ReasoningEffort),
		IncludeReasoning: &req.IncludeReasoning,
	}

	if req.ResponseFormat != "" {
		wireReq.ResponseFormat = &wireResponseFormat{
			Type: req.ResponseFormat,
		}
	}

	payload, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("llm: failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("llm: failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llm: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	duration := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("llm: failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: unexpected status %s: %s", resp.Status, string(body))
	}

	var wireResp wireResponse
	if err := json.Unmarshal(body, &wireResp); err != nil {
		return nil, fmt.Errorf("llm: failed to decode response JSON: %w", err)
	}

	if wireResp.Error != nil {
		return nil, fmt.Errorf("llm: API returned error: %s (type: %s)", wireResp.Error.Message, wireResp.Error.Type)
	}

	if len(wireResp.Choices) == 0 {
		return nil, fmt.Errorf("llm: received empty choices in response: %s", string(body))
	}

	choice := wireResp.Choices[0]
	thinkingText, cleanContent := extractThinking(choice)
	cleanContent = cleanCodeFences(cleanContent)

	return &ChatResponse{
		Content:          cleanContent,
		ReasoningContent: thinkingText,
		Usage: Usage{
			PromptTokens:     wireResp.Usage.PromptTokens,
			CompletionTokens: wireResp.Usage.CompletionTokens,
			ReasoningTokens:  wireResp.Usage.CompletionTokensDetails.ReasoningTokens,
			TotalTokens:      wireResp.Usage.TotalTokens,
		},
		Duration: duration,
	}, nil
}

// cleanCodeFences strips optional markdown ``` or ```json wrappers.
func cleanCodeFences(s string) string {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "```json") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
	} else if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```")
	}
	if strings.HasSuffix(trimmed, "```") {
		trimmed = strings.TrimSuffix(trimmed, "```")
	}
	return strings.TrimSpace(trimmed)
}
