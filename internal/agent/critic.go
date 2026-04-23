package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/arthurlw/ouroboros/internal/sandbox"
)

// StandardCritic evaluates the results from the sandbox.
type StandardCritic struct{}

func NewCritic() *StandardCritic {
	return &StandardCritic{}
}

// Verify analyzes test results and returns (passed, failureKind, feedback)
func (c *StandardCritic) Verify(ctx context.Context, res *sandbox.Result) (bool, sandbox.FailureKind, string) {
	if res.ExitCode == 0 {
		return true, sandbox.FailureNone, "Tests passed."
	}

	// Check for timeout first (may be set by runContainer)
	if res.FailureKind == sandbox.FailureTimeout {
		slog.Error("Detected TIMEOUT")
		return false, sandbox.FailureTimeout, "EXECUTION TIMEOUT"
	}

	// Analyze failure types
	if strings.Contains(res.Stderr, "build failed") {
		slog.Error("Detected BUILD FAILURE")
		res.FailureKind = sandbox.FailureBuild
		return false, sandbox.FailureBuild, fmt.Sprintf("COMPILER ERROR:\n%s", res.Stderr)
	}

	if strings.Contains(res.Stdout, "FAIL:") || strings.Contains(res.Stdout, "--- FAIL") {
		slog.Error("Detected TEST FAILURE")
		res.FailureKind = sandbox.FailureTest
		return false, sandbox.FailureTest, fmt.Sprintf("TEST FAILURE:\n%s", res.Stdout)
	}

	res.FailureKind = sandbox.FailureUnknown
	return false, sandbox.FailureUnknown, fmt.Sprintf("UNKNOWN ERROR (Exit %d):\n%s\n%s", res.ExitCode, res.Stdout, res.Stderr)
}
