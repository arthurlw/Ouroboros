package agent

import (
	"context"
	"fmt"

	"github.com/arthurlw/ouroboros/internal/llm"
)

// LLMProvider is the interface used throughout the agent package
// It provides backward compatibility with the new llm package
type LLMProvider interface {
	Generate(ctx context.Context, prompt string, systemPrompt ...string) (string, error)
}

// LLMAdapter adapts the new llm.Provider to the legacy LLMProvider interface
type LLMAdapter struct {
	provider llm.Provider
}

// NewClient creates a new LLM client using the new provider system
func NewClient() *LLMAdapter {
	provider, err := llm.NewProvider()
	if err != nil {
		panic(fmt.Sprintf("failed to create LLM provider: %v", err))
	}
	return &LLMAdapter{provider: provider}
}

// Generate implements the legacy interface
func (a *LLMAdapter) Generate(ctx context.Context, prompt string, systemPrompt ...string) (string, error) {
	opts := []llm.GenerateOption{}

	if len(systemPrompt) > 0 && systemPrompt[0] != "" {
		opts = append(opts, llm.WithSystemPrompt(systemPrompt[0]))
	}

	resp, err := a.provider.Generate(ctx, prompt, opts...)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}
