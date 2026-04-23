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

// OllamaProvider implements the Provider interface for local Ollama
type OllamaProvider struct {
	DefaultModel string
	BaseURL     string
	HTTPClient  *http.Client
}

// NewOllamaProvider creates a new Ollama provider
func NewOllamaProvider() *OllamaProvider {
	return &OllamaProvider{
		DefaultModel: "qwen2.5-coder:7b",
		BaseURL:     "http://localhost:11434/v1/chat/completions",
		HTTPClient:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (o *OllamaProvider) Name() string {
	return "ollama"
}

func (o *OllamaProvider) Generate(ctx context.Context, prompt string, opts ...GenerateOption) (Response, error) {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	model := o.DefaultModel
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
	req, err := http.NewRequestWithContext(ctx, "POST", o.BaseURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return Response{}, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := o.HTTPClient.Do(req)
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
