package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnthropicProvider implements the Provider interface for Anthropic Claude
type AnthropicProvider struct {
	APIKey      string
	DefaultModel string
	BaseURL     string
	HTTPClient  *http.Client
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(apiKey string) *AnthropicProvider {
	return &AnthropicProvider{
		APIKey:      apiKey,
		DefaultModel: "claude-sonnet-4-20250514",
		BaseURL:     "https://api.anthropic.com/v1/messages",
		HTTPClient:  &http.Client{Timeout: 90 * time.Second},
	}
}

func (a *AnthropicProvider) Name() string {
	return "anthropic"
}

func (a *AnthropicProvider) Generate(ctx context.Context, prompt string, opts ...GenerateOption) (Response, error) {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	model := a.DefaultModel
	if options.Model != "" {
		model = options.Model
	}

	maxTokens := 4096
	if options.MaxTokens > 0 {
		maxTokens = options.MaxTokens
	}

	payload := map[string]interface{}{
		"model":      model,
		"max_tokens": maxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": options.Temperature,
	}

	// Anthropic uses system parameter separately
	if options.SystemPrompt != "" {
		payload["system"] = options.SystemPrompt
	}

	jsonPayload, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", a.BaseURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return Response{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return Response{}, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Model       string `json:"model"`
		StopReason  string `json:"stop_reason"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Response{}, err
	}

	if len(result.Content) == 0 {
		return Response{}, fmt.Errorf("empty response from AI")
	}

	return Response{
		Content:      result.Content[0].Text,
		PromptTokens: result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
		Model:        result.Model,
		FinishReason: result.StopReason,
	}, nil
}
