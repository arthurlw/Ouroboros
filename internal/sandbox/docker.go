package sandbox

import (
	"context"
	"fmt"
	"time"
	// ... imports from before
)

type Client struct {
	// ... same as before
}

// ExecuteTest runs the standard "go test" suite.
func (s *Client) ExecuteTest(ctx context.Context, code string, test string) (*Result, error) {
	// 1. Write files to the staging directory (Implementation skipped for brevity)
	// s.writeToStaging("main.go", code)
	// s.writeToStaging("main_test.go", test)

	// 2. Execute with strict timeout
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return s.runContainer(ctx, []string{"go", "test", "-v", "./..."})
}

// ExecuteBenchmark runs "go test -bench" for Optimization Mode.
func (s *Client) ExecuteBenchmark(ctx context.Context, code string) (*Result, error) {
	// Higher timeout for benchmarks
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// We look for performance improvements (CPU/Memory op)
	return s.runContainer(ctx, []string{"go", "test", "-bench=.", "-benchmem", "./..."})
}

// runContainer is the internal helper (refactored from previous "Execute")
func (s *Client) runContainer(ctx context.Context, cmd []string) (*Result, error) {
	// ... (Same Docker API logic as previous response, ensuring NetworkMode="none")
	// The key is returning the *Result struct with raw Stdout/Stderr for the Critic.
	return &Result{}, nil
}
