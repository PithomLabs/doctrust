package compiler

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

type LLMClient interface {
	Generate(ctx context.Context, prompt string) (string, error)
	GenerateWithSystem(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

type OpenRouterClient struct {
	httpClient *http.Client
	apiKey     string
	model      string
}

type openRouterRequest struct {
	Model       string            `json:"model"`
	Messages    []openRouterMsg   `json:"messages"`
	ResponseFmt *responseFormat   `json:"response_format,omitempty"`
}

type openRouterMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func NewOpenRouterClient() *OpenRouterClient {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = "anthropic/claude-sonnet-4-20250514"
	}

	return &OpenRouterClient{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		apiKey:     apiKey,
		model:      model,
	}
}

func (c *OpenRouterClient) Generate(ctx context.Context, prompt string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY not set")
	}

	systemPrompt := `You are a policy compiler. You convert structured policy definitions into OPA/Rego.

You must produce valid JSON with two fields:
- "policy_rego": the complete Rego policy module
- "policy_test_rego": supplementary OPA unit tests

Constraints:
- Package must be: package doctrust.policy
- Must import rego.v1
- Must produce a result rule with decision + findings
- Decision must be one of: PASS, FAIL, REVIEW, MISSING_EVIDENCE
- Never produce EVALUATION_ERROR (that is a system state)
- Must preserve the exact evidence schema and semantic types provided
- Do not invent fields or semantic types not in the schema`

	reqBody := openRouterRequest{
		Model: c.model,
		Messages: []openRouterMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		ResponseFmt: &responseFormat{Type: "json_object"},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
			}
		}

		result, err := c.doRequest(ctx, body)
		if err != nil {
			lastErr = err
			if isRetryableError(err) {
				continue
			}
			return "", err
		}
		return result, nil
	}

	return "", fmt.Errorf("LLM failed after 3 attempts: %w", lastErr)
}

// GenerateWithSystem sends an explicit system prompt and user prompt.
func (c *OpenRouterClient) GenerateWithSystem(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY not set")
	}

	reqBody := openRouterRequest{
		Model: c.model,
		Messages: []openRouterMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		ResponseFmt: &responseFormat{Type: "json_object"},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
			}
		}

		result, err := c.doRequest(ctx, body)
		if err != nil {
			lastErr = err
			if isRetryableError(err) {
				continue
			}
			return "", err
		}
		return result, nil
	}

	return "", fmt.Errorf("LLM failed after 3 attempts: %w", lastErr)
}

func (c *OpenRouterClient) doRequest(ctx context.Context, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == 429 {
		return "", fmt.Errorf("rate limited (HTTP 429)")
	}
	if resp.StatusCode >= 500 {
		return "", fmt.Errorf("server error (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp openRouterResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("API error: %s", apiResp.Error.Message)
	}

	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return apiResp.Choices[0].Message.Content, nil
}

// isRetryableError determines if an error is transient and worth retrying.
func isRetryableError(err error) bool {
	errStr := err.Error()
	// HTTP retryable status codes
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") || strings.Contains(errStr, "503") {
		return true
	}
	// Network errors (connection refused, reset, etc.)
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "EOF") ||
		strings.Contains(errStr, "i/o timeout") {
		return true
	}
	return false
}
