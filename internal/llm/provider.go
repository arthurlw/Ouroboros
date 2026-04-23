package llm

import "context"

// Provider is the interface all LLM providers must implement
type Provider interface {
	Generate(ctx context.Context, prompt string, opts ...GenerateOption) (Response, error)
	Name() string
}

// Response contains the LLM's response and metadata
type Response struct {
	Content      string
	PromptTokens int
	OutputTokens int
	Model        string
	FinishReason string
}

// GenerateOptions configures LLM generation
type GenerateOptions struct {
	SystemPrompt string
	Temperature  float64
	MaxTokens    int
	Model        string
}

// GenerateOption is a functional option for Generate
type GenerateOption func(*GenerateOptions)

// WithSystemPrompt sets a custom system prompt
func WithSystemPrompt(prompt string) GenerateOption {
	return func(o *GenerateOptions) {
		o.SystemPrompt = prompt
	}
}

// WithTemperature sets the sampling temperature
func WithTemperature(temp float64) GenerateOption {
	return func(o *GenerateOptions) {
		o.Temperature = temp
	}
}

// WithMaxTokens sets the maximum output tokens
func WithMaxTokens(tokens int) GenerateOption {
	return func(o *GenerateOptions) {
		o.MaxTokens = tokens
	}
}

// WithModel overrides the default model
func WithModel(model string) GenerateOption {
	return func(o *GenerateOptions) {
		o.Model = model
	}
}

// DefaultOptions returns sensible defaults
func DefaultOptions() GenerateOptions {
	return GenerateOptions{
		SystemPrompt: "You are Ouroboros, a Golang systems architect. Follow the user's output-format instructions precisely.",
		Temperature:  0.1,
		MaxTokens:    4096,
	}
}
