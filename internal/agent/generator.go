package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings" // This is now used by extractBlock below
)

type Generator struct {
	LLM LLMProvider
}

func NewGenerator(llm LLMProvider) *Generator {
	return &Generator{LLM: llm}
}

// GenerateCodeAndTest accepts optional 'existingCode' for refactoring
func (g *Generator) GenerateCodeAndTest(ctx context.Context, requirement string, existingCode string, existingTest string) (string, string, error) {
	slog.Info("🟣 GENERATING CODE", "requirement", requirement, "is_refactor", existingCode != "")

	var prompt string
	if existingCode == "" {
		// MODE A: NEW TOOL (Write from scratch)
		prompt = fmt.Sprintf(`
TASK: Write a GO program and TEST SUITE.
REQUIREMENT: %s

INSTRUCTIONS:
1. Both files must start with 'package main'.
2. The code must be self-contained.
3. Be careful with imports.

OUTPUT FORMAT:
Block 1: implementation (markdown 'go')
Block 2: test (markdown 'go')
`, requirement)
	} else {
		// MODE B: REFACTOR (Update existing)
		prompt = fmt.Sprintf(`
TASK: Refactor this GO program and TEST SUITE.
REQUIREMENT: %s

EXISTING IMPLEMENTATION:
%s

EXISTING TEST:
%s

INSTRUCTIONS:
1. Modify the existing code to satisfy the NEW REQUIREMENT.
2. Keep existing functionality working if possible.
3. Ensure 'package main'.

OUTPUT FORMAT:
Block 1: New implementation (markdown 'go')
Block 2: New test (markdown 'go')
`, requirement, existingCode, existingTest)
	}

	raw, err := g.LLM.Generate(ctx, prompt)
	if err != nil { return "", "", err }

	code := extractBlock(raw, "go")
	test := extractBlock(raw, "go", "test")

	// Fallback: If LLM didn't return a test block, keep the old one
	if test == "" && existingTest != "" {
		test = existingTest
	}

	if code == "" {
		return "", "", fmt.Errorf("LLM failed to generate code block")
	}

	return code, test, nil
}

// --- SHARED HELPER FUNCTIONS ---

// extractBlock parses markdown code fences.
// It is available to the entire 'agent' package (agent.go can use this too).
func extractBlock(content string, lang string, keywordHint ...string) string {
	parts := strings.Split(content, "```"+lang)
	if len(parts) < 2 {
		parts = strings.Split(content, "```")
	}

	if len(parts) < 2 {
		return ""
	}

	for _, part := range parts[1:] {
		code := strings.Split(part, "```")[0]
		if len(keywordHint) > 0 {
			if strings.Contains(code, keywordHint[0]) || strings.Contains(code, "testing") {
				return strings.TrimSpace(code)
			}
		} else {
			// If looking for implementation, try to avoid the test file
			if !strings.Contains(code, "testing") {
				return strings.TrimSpace(code)
			}
		}
	}
	// Fallback to first block
	return strings.TrimSpace(strings.Split(parts[1], "```")[0])
}
