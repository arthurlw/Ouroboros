package agent

import (
	"context"
	"fmt"
	"log/slog"
	"ouroboros/internal/sandbox"
	"strings"
)

// StandardCritic evaluates the results from the Gym.
type StandardCritic struct{}

func NewCritic() *StandardCritic {
	return &StandardCritic{}
}

func (c *StandardCritic) Verify(ctx context.Context, res *sandbox.Result) (bool, string) {
	if res.ExitCode == 0 {
		return true, "Tests passed."
	}

	// Analyze failure
	if strings.Contains(res.Stderr, "build failed") {
		slog.Error("Detected BUILD FAILURE")
		return false, fmt.Sprintf("COMPILER ERROR:\n%s", res.Stderr)
	}

	if strings.Contains(res.Stdout, "FAIL:") || strings.Contains(res.Stdout, "--- FAIL") {
		slog.Error("Detected TEST FAILURE")
		return false, fmt.Sprintf("TEST FAILURE:\n%s", res.Stdout)
	}

	if res.ExitCode == 124 {
		slog.Error("Detected TIMEOUT")
		return false, "EXECUTION TIMEOUT"
	}

	return false, fmt.Sprintf("UNKNOWN ERROR (Exit %d):\n%s\n%s", res.ExitCode, res.Stdout, res.Stderr)
}
