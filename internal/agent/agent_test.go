package agent

import (
	"testing"
)

// TestEvolveTool_TimeoutOnLastIteration would require mocking Sandbox,
// which is a larger refactor. For now, the regression is guarded by:
//   1. The explicit isLastIteration check inside the timeout branch
//   2. The defensive final return fmt.Errorf(...) that makes silent-success impossible
// A full mock-based test should be added when Sandbox is extracted to an interface.
func TestEvolveTool_DefensiveReturnExists(t *testing.T) {
	// This test documents intent; real coverage comes after Sandbox becomes an interface.
	t.Log("evolveTool must return a non-nil error if the loop exits without verification")
}
