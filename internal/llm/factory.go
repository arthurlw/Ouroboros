package llm

import (
	"fmt"
	"log/slog"
	"os"
)

// NewProvider creates a provider based on environment variables with retry logic
// Priority: ANTHROPIC_API_KEY > GROQ_API_KEY > OPENAI_API_KEY > Ollama (local)
func NewProvider() (Provider, error) {
	var baseProvider Provider

	// 1. Check for Anthropic
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		slog.Info("🧠 Using Provider: ANTHROPIC (Claude Sonnet 4)")
		baseProvider = NewAnthropicProvider(key)
	} else if key := os.Getenv("GROQ_API_KEY"); key != "" {
		// 2. Check for Groq
		slog.Info("🧠 Using Provider: GROQ (Llama 3.3 70B)")
		baseProvider = NewGroqProvider(key)
	} else if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		// 3. Check for OpenAI
		slog.Info("🧠 Using Provider: OPENAI (GPT-4o)")
		baseProvider = NewOpenAIProvider(key)
	} else {
		// 4. Fallback to Local (Ollama)
		slog.Info("🧠 Using Provider: OLLAMA (Local - Qwen 2.5 Coder)")
		baseProvider = NewOllamaProvider()
	}

	// Wrap with retry logic
	return NewRetryProvider(baseProvider, DefaultRetryConfig()), nil
}

// NewProviderByName creates a specific provider by name
func NewProviderByName(name string) (Provider, error) {
	switch name {
	case "anthropic":
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
		}
		return NewAnthropicProvider(key), nil
	case "groq":
		key := os.Getenv("GROQ_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("GROQ_API_KEY not set")
		}
		return NewGroqProvider(key), nil
	case "openai":
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY not set")
		}
		return NewOpenAIProvider(key), nil
	case "ollama":
		return NewOllamaProvider(), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}
