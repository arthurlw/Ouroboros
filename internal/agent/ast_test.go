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
	orchestratorBlock := extractFuncBlock(summary, "func Orchestrator()")
	if !strings.Contains(orchestratorBlock, "↳ Calls:") {
		t.Errorf("Orchestrator function missing 'Calls' block")
	}
}

func TestParseFile_DeepInspection(t *testing.T) {
	// 1. Setup: Create a single file with complex dependency logic
	tmpDir := t.TempDir()
	code := `
package complex

import (
	"fmt"
	"net/http"
)

// Controller calls Service and Logger
func Controller() {
	Service()
	fmt.Println("Done")
}

// Service calls Repository and External API
func Service() {
	Repository()
	http.Get("google.com")
}

func Repository() {
	// Leaf node
}
`
	path := filepath.Join(tmpDir, "complex.go")
	if err := os.WriteFile(path, []byte(code), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Execution: Call the PRIVATE function 'parseFile' directly
	// We can do this because we are in 'package agent' (white-box testing)
	skel, err := parseFile(path)
	if err != nil {
		t.Fatalf("parseFile failed: %v", err)
	}

	// 3. Verification: Check the struct fields directly (No string parsing!)

	// A. Check Package Name
	if skel.Package != "complex" {
		t.Errorf("Expected package 'complex', got '%s'", skel.Package)
	}

	// B. Check Function Discovery
	if len(skel.Functions) != 3 {
		t.Errorf("Expected 3 functions, got %d", len(skel.Functions))
	}

	// C. Check Dependency Graph (The "Math" Part)
	// We expect: Controller -> [Service, fmt.Println]
	deps := skel.Dependencies["Controller"]

	// Helper to check slice containment
	assertContains := func(list []string, target string) {
		found := false
		for _, item := range list {
			if item == target {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Controller dependency list missing '%s'. Got: %v", target, list)
		}
	}

	assertContains(deps, "Service")
	assertContains(deps, "fmt.Println")

	// D. Check Nested Dependencies
	// We expect: Service -> [Repository, http.Get]
	serviceDeps := skel.Dependencies["Service"]
	assertContains(serviceDeps, "http.Get")
	assertContains(serviceDeps, "Repository")
}

// Helper to find the text block for a specific function to ensure we aren't matching
// calls from the wrong function.
func extractFuncBlock(fullText, funcHeader string) string {
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
