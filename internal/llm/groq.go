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

// GroqProvider implements the Provider interface for Groq
type GroqProvider struct {
	APIKey      string
	DefaultModel string
	BaseURL     string
	HTTPClient  *http.Client
}

// NewGroqProvider creates a new Groq provider
func NewGroqProvider(apiKey string) *GroqProvider {
	return &GroqProvider{
		APIKey:      apiKey,
		DefaultModel: "llama-3.3-70b-versatile",
		BaseURL:     "https://api.groq.com/openai/v1/chat/completions",
		HTTPClient:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (g *GroqProvider) Name() string {
	return "groq"
}

func (g *GroqProvider) Generate(ctx context.Context, prompt string, opts ...GenerateOption) (Response, error) {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	model := g.DefaultModel
	if options.Model != "" {
		model = options.Model
	}

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": options.SystemPrompt},
			{"role": "user", "content": prompt},
		},
		"temperature": options.Temperature,
	}

	if options.MaxTokens > 0 {
		payload["max_tokens"] = options.MaxTokens
	}

	jsonPayload, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", g.BaseURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return Response{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.APIKey)

	resp, err := g.HTTPClient.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return Response{}, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Model string `json:"model"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Response{}, err
	}

	if len(result.Choices) == 0 {
		return Response{}, fmt.Errorf("empty response from AI")
	}

	return Response{
		Content:      result.Choices[0].Message.Content,
		PromptTokens: result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
		Model:        result.Model,
		FinishReason: result.Choices[0].FinishReason,
	}, nil
}
