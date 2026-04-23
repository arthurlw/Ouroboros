package llm

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
)

// RetryConfig configures retry behavior
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// DefaultRetryConfig returns sensible retry defaults
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   8 * time.Second,
	}
}

// RetryProvider wraps a provider with exponential backoff retry logic
type RetryProvider struct {
	provider Provider
	config   RetryConfig
}

// NewRetryProvider wraps a provider with retry logic
func NewRetryProvider(provider Provider, config RetryConfig) *RetryProvider {
	return &RetryProvider{
		provider: provider,
		config:   config,
	}
}

func (r *RetryProvider) Name() string {
	return fmt.Sprintf("%s-with-retry", r.provider.Name())
}

func (r *RetryProvider) Generate(ctx context.Context, prompt string, opts ...GenerateOption) (Response, error) {
	var lastErr error

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := r.calculateBackoff(attempt)
			slog.Info("🔄 Retrying LLM call", "attempt", attempt, "delay", delay, "provider", r.provider.Name())

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return Response{}, ctx.Err()
			}
		}

		resp, err := r.provider.Generate(ctx, prompt, opts...)
		if err == nil {
			if attempt > 0 {
				slog.Info("✅ LLM retry successful", "attempt", attempt)
			}
			return resp, nil
		}

		lastErr = err

		// Check if error is retryable
		if !r.isRetryable(err) {
			slog.Warn("❌ Non-retryable LLM error", "error", err)
			return Response{}, err
		}

		if attempt < r.config.MaxRetries {
			slog.Warn("⚠️ LLM call failed, will retry", "error", err, "attempt", attempt+1, "max", r.config.MaxRetries)
		}
	}

	return Response{}, fmt.Errorf("max retries exceeded (%d attempts): %w", r.config.MaxRetries+1, lastErr)
}

// calculateBackoff returns exponential backoff delay: baseDelay * 2^(attempt-1)
func (r *RetryProvider) calculateBackoff(attempt int) time.Duration {
	delay := time.Duration(float64(r.config.BaseDelay) * math.Pow(2, float64(attempt-1)))
	if delay > r.config.MaxDelay {
		delay = r.config.MaxDelay
	}
	return delay
}

// isRetryable determines if an error should be retried
func (r *RetryProvider) isRetryable(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Retry on rate limits (429)
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "rate limit") {
		return true
	}

	// Retry on server errors (5xx)
	if strings.Contains(errStr, "500") || strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") || strings.Contains(errStr, "504") {
		return true
	}

	// Retry on connection errors
	if strings.Contains(errStr, "connection") || strings.Contains(errStr, "timeout") {
		return true
	}

	// Don't retry on client errors (4xx except 429)
	if strings.Contains(errStr, "400") || strings.Contains(errStr, "401") ||
		strings.Contains(errStr, "403") || strings.Contains(errStr, "404") {
		return false
	}

	// Default: retry on unknown errors (safer)
	return true
}

// IsRateLimitError checks if an HTTP status code is a rate limit error
func IsRateLimitError(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests
}

// IsServerError checks if an HTTP status code is a server error
func IsServerError(statusCode int) bool {
	return statusCode >= 500 && statusCode < 600
}
