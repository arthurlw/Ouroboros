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

// NewClient creates a new LLM client using the provider auto-detection logic.
// Returns an error if no provider can be constructed (e.g., all constructors fail).
func NewClient() (*LLMAdapter, error) {
	provider, err := llm.NewProvider()
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM provider: %w", err)
	}
	return &LLMAdapter{provider: provider}, nil
}

// NewClientFromProvider wraps an explicit provider (useful for tests and --provider flag).
func NewClientFromProvider(p llm.Provider) *LLMAdapter {
	return &LLMAdapter{provider: p}
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
