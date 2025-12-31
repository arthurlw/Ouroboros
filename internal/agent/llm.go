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

type LLMProvider interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

type Client struct {
	APIKey  string
	Model   string
	BaseURL string
}

func NewClient() *Client {
	// 1. Check for Groq
	if key := os.Getenv("GROQ_API_KEY"); key != "" {
		slog.Info("🧠 Using Provider: GROQ")
		return &Client{
			APIKey:  key,
			Model:   "llama-3.3-70b-versatile",
			BaseURL: "https://api.groq.com/openai/v1/chat/completions",
		}
	}

	// 2. Check for OpenAI
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		slog.Info("🧠 Using Provider: OPENAI")
		return &Client{
			APIKey:  key,
			Model:   "gpt-4o",
			BaseURL: "[https://api.openai.com/v1/chat/completions](https://api.openai.com/v1/chat/completions)",
		}
	}

	// 3. Fallback to Local (Ollama)
	slog.Info("🧠 Using Provider: OLLAMA (Local)")
	return &Client{
		APIKey:  "ollama",
		Model:   "qwen2.5-coder:7b",
		BaseURL: "http://localhost:11434/v1/chat/completions",
	}
}

func (c *Client) Generate(ctx context.Context, prompt string) (string, error) {
	payload := map[string]interface{}{
		"model": c.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are Ouroboros, an expert Golang Systems Architect. RETURN ONLY JSON."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.1,
	}

	jsonPayload, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("connection failed: %w", err)
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
		return "", fmt.Errorf("empty response from AI")
	}

	return result.Choices[0].Message.Content, nil
}
