package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSummarizeAgent_CallGraph(t *testing.T) {
	// 1. Create a temporary directory for the "dummy" project
	tmpDir := t.TempDir()

	// 2. Create a sample Go file with known dependencies
	// Scenario: 'Orchestrator' calls 'Worker' and 'fmt.Println'
	dummyCode := `
package dummy

import (
	"fmt"
	"os"
)

func Orchestrator() {
	fmt.Println("Starting work...")
	data := os.Getenv("DATA")
	Worker(data)
}

func Worker(data string) {
	// Do nothing
}
`
	dummyPath := filepath.Join(tmpDir, "dummy.go")
	if err := os.WriteFile(dummyPath, []byte(dummyCode), 0644); err != nil {
		t.Fatalf("Failed to create dummy file: %v", err)
	}

	// 3. Run the Summarizer on this temporary directory
	summary, err := SummarizeAgent(tmpDir)
	if err != nil {
		t.Fatalf("SummarizeAgent failed: %v", err)
	}

	// 4. VERIFICATION
	// We check if the output contains the specific "Calls:" signatures we expect.

	t.Log("--- Captured Summary Output ---")
	t.Log(summary)

	// Check 1: Does it find the functions?
	if !strings.Contains(summary, "func Orchestrator()") {
		t.Errorf("Summary missing 'Orchestrator' function definition")
	}

	// Check 2: Does it detect external calls (fmt, os)?
	if !strings.Contains(summary, "fmt.Println") {
		t.Errorf("Failed to detect dependency: fmt.Println")
	}
	if !strings.Contains(summary, "os.Getenv") {
		t.Errorf("Failed to detect dependency: os.Getenv")
	}

	// Check 3: Does it detect internal calls (Worker)?
	if !strings.Contains(summary, "Worker") {
		t.Errorf("Failed to detect internal dependency: Worker")
	}

	// Check 4: (Optional) Verify specific structure
	// We expect a line like: "↳ Calls: [Worker, fmt.Println, os.Getenv]"
	// Since order can vary, we just ensure the block exists under Orchestrator.
	orchestratorBlock := extractBlock(summary, "func Orchestrator()")
	if !strings.Contains(orchestratorBlock, "↳ Calls:") {
		t.Errorf("Orchestrator function missing 'Calls' block")
	}
}

// Helper to find the text block for a specific function to ensure we aren't matching
// calls from the wrong function.
func extractBlock(fullText, funcHeader string) string {
	parts := strings.Split(fullText, funcHeader)
	if len(parts) < 2 {
		return ""
	}
	// Return the text immediately following the header up to the next double-newline
	rest := parts[1]
	if idx := strings.Index(rest, "\n\n"); idx != -1 {
		return rest[:idx]
	}
	return rest
}
