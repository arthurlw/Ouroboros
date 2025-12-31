package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// LLMProvider defines how we get text from models.
type LLMProvider interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// OpenAIClient implements LLMProvider for OpenAI/Anthropic/compatible APIs.
type OpenAIClient struct {
	APIKey string
	Model  string
	URL    string
}

func NewOpenAIClient(apiKey string) *OpenAIClient {
	return &OpenAIClient{
		APIKey: apiKey,
		Model:  "gpt-4o", // Use a smart model for architecture tasks
		URL:    "https://api.openai.com/v1/chat/completions",
	}
}

func (c *OpenAIClient) Generate(ctx context.Context, prompt string) (string, error) {
	requestBody, _ := json.Marshal(map[string]interface{}{
		"model": c.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are Ouroboros, an expert Golang Systems Architect."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2, // Low temperature for code determinism
	})

	req, err := http.NewRequestWithContext(ctx, "POST", c.URL, bytes.NewBuffer(requestBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty response from LLM")
	}

	return result.Choices[0].Message.Content, nil
}
