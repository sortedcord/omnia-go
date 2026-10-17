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
	"sync"
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

// WithSchemaFallback enables or disables automatic fallback to json_object + prompt rendering
// when a provider rejects json_schema with HTTP 400.
func WithSchemaFallback(enable bool) Option {
	return func(client *HTTPClient) {
		client.enableFallback = enable
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
	endpoint                  string
	apiKey                    string
	httpClient                *http.Client
	fallbackMu                sync.RWMutex
	disableJSONSchema         bool
	consecutiveSchemaFailures int
	schemaFailureThreshold    int
	enableFallback            bool
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
		enableFallback:         true,
		schemaFailureThreshold: 2,
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// ChatCompletion sends a request to the chat completions API and returns the parsed result.
func (c *HTTPClient) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	c.fallbackMu.RLock()
	disabled := c.disableJSONSchema
	c.fallbackMu.RUnlock()

	isSchema := req.ResponseFormat != nil && req.ResponseFormat.Type == "json_schema"

	if disabled && isSchema {
		fallbackReq := c.makeFallbackRequest(req)
		resp, _, err := c.doRoundTripWithStatus(ctx, fallbackReq)
		return resp, err
	}

	resp, statusCode, err := c.doRoundTripWithStatus(ctx, req)
	if err == nil {
		if isSchema {
			c.fallbackMu.Lock()
			c.consecutiveSchemaFailures = 0
			c.fallbackMu.Unlock()
		}
		return resp, nil
	}

	// D8 Fallback: if HTTP 400 on json_schema and fallback is enabled, retry once
	if isSchema && c.enableFallback && statusCode == http.StatusBadRequest {
		fallbackReq := c.makeFallbackRequest(req)
		fallbackResp, _, fallbackErr := c.doRoundTripWithStatus(ctx, fallbackReq)
		if fallbackErr == nil {
			c.fallbackMu.Lock()
			c.consecutiveSchemaFailures++
			if c.consecutiveSchemaFailures >= c.schemaFailureThreshold {
				c.disableJSONSchema = true
			}
			c.fallbackMu.Unlock()
			return fallbackResp, nil
		}
	}

	return nil, err
}

func (c *HTTPClient) makeFallbackRequest(req ChatRequest) ChatRequest {
	fb := req
	fb.ResponseFormat = JSONObject()

	if req.ResponseFormat != nil && req.ResponseFormat.JSONSchema != nil && req.ResponseFormat.JSONSchema.Schema != nil {
		prompt := "EXPECTED JSON OUTPUT SCHEMA:\n" + req.ResponseFormat.JSONSchema.Schema.PromptText()
		fb.Messages = make([]Message, len(req.Messages)+1)
		copy(fb.Messages, req.Messages)
		fb.Messages[len(req.Messages)] = Message{
			Role:    System,
			Content: prompt,
		}
	}
	return fb
}

func (c *HTTPClient) doRoundTripWithStatus(ctx context.Context, req ChatRequest) (*ChatResponse, int, error) {
	wireReq := wireRequest{
		Model:            req.Model,
		Messages:         req.Messages,
		ResponseFormat:   req.ResponseFormat,
		ReasoningEffort:  string(req.ReasoningEffort),
		IncludeReasoning: &req.IncludeReasoning,
	}

	payload, err := json.Marshal(wireReq)
	if err != nil {
		return nil, 0, fmt.Errorf("llm: failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewBuffer(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("llm: failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("llm: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	duration := time.Since(start)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("llm: failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("llm: unexpected status %s: %s", resp.Status, string(body))
	}

	var wireResp wireResponse
	if err := json.Unmarshal(body, &wireResp); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("llm: failed to decode response JSON: %w", err)
	}

	if wireResp.Error != nil {
		return nil, resp.StatusCode, fmt.Errorf("llm: API returned error: %s (type: %s)", wireResp.Error.Message, wireResp.Error.Type)
	}

	if len(wireResp.Choices) == 0 {
		return nil, resp.StatusCode, fmt.Errorf("llm: received empty choices in response: %s", string(body))
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
	}, resp.StatusCode, nil
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
