package agent

import (
	"context"
	"log/slog"
	"ouroboros/internal/sandbox"
	"regexp"
	"strings"
)

// StandardCritic evaluates the results from the Gym.
type StandardCritic struct{}

func NewCritic() *StandardCritic {
	return &StandardCritic{}
}

// Verify analyzes the execution result.
// Returns: (Passed?, FeedbackString)
func (c *StandardCritic) Verify(ctx context.Context, res *sandbox.Result) (bool, string) {
	logger := slog.With("component", "critic", "exit_code", res.ExitCode)

	// 1. The Binary Check (Exit Code)
	if res.ExitCode == 0 {
		logger.Info("🟢 VERIFICATION PASSED", "stdout_len", len(res.Stdout))
		return true, "Tests passed successfully."
	}

	// 2. The Forensic Analysis (Why did it fail?)
	logger.Warn("🔴 VERIFICATION FAILED", "stderr_len", len(res.Stderr))

	// Helper: Check for common failure modes
	if strings.Contains(res.Stderr, "build failed") {
		logger.Error("Detected BUILD FAILURE")
		return false, fmt.Sprintf("COMPILER ERROR:\n%s", c.extractCompilerError(res.Stderr))
	}

	if strings.Contains(res.Stdout, "FAIL:") || strings.Contains(res.Stdout, "--- FAIL") {
		logger.Error("Detected TEST FAILURE")
		return false, fmt.Sprintf("TEST FAILURE:\n%s", c.extractTestFailure(res.Stdout))
	}

	if res.ExitCode == 124 { // Standard timeout exit code
		logger.Error("Detected TIMEOUT")
		return false, "EXECUTION TIMEOUT: The code took too long to run. Check for infinite loops."
	}

	// Fallback
	return false, fmt.Sprintf("UNKNOWN ERROR (Exit %d):\n%s\n%s", res.ExitCode, res.Stdout, res.Stderr)
}

// extractCompilerError attempts to grab only the relevant lines from a verbose Go build log.
func (c *StandardCritic) extractCompilerError(log string) string {
	// CRITICAL EVALUATION: Regex is fragile.
	// If the Go compiler output format changes, this breaks.
	// For Ouroboros v1, we just return the last 10 lines as a heuristic.
	lines := strings.Split(log, "\n")
	if len(lines) > 10 {
		return strings.Join(lines[len(lines)-10:], "\n")
	}
	return log
}

// extractTestFailure attempts to find the specific assertion that failed.
func (c *StandardCritic) extractTestFailure(log string) string {
	// Simple heuristic: Look for lines starting with "\t" (Go testing usually indents errors)
	var meaningfulLines []string
	lines := strings.Split(log, "\n")
	capture := false

	for _, line := range lines {
		if strings.Contains(line, "FAIL") {
			capture = true
		}
		if capture {
			meaningfulLines = append(meaningfulLines, line)
		}
	}

	if len(meaningfulLines) == 0 {
		return log // Fallback to raw log if parsing fails
	}
	return strings.Join(meaningfulLines, "\n")
}
