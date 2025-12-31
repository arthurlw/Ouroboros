package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// Generator is responsible for writing raw code.
type Generator struct {
	LLM LLMProvider
}

func NewGenerator(llm LLMProvider) *Generator {
	return &Generator{LLM: llm}
}

// GenerateCodeAndTest asks the LLM to write implementation and verification code.
func (g *Generator) GenerateCodeAndTest(ctx context.Context, requirement string) (string, string, error) {
	slog.Info("🟣 GENERATING CODE", "requirement", requirement)

	prompt := fmt.Sprintf(`
TASK: Write a GO program and a corresponding TEST SUITE.
REQUIREMENT: %s

OUTPUT FORMAT:
You must provide two distinct blocks of code wrapped in markdown.
Block 1: The implementation (package main)
Block 2: The test (package main_test)

CONSTRAINT:
- Use standard library only where possible.
- The code must be self-contained.
`, requirement)

	raw, err := g.LLM.Generate(ctx, prompt)
	if err != nil {
		return "", "", err
	}

	// Helper to extract code blocks from Markdown
	code := extractBlock(raw, "go")
	test := extractBlock(raw, "go", "test")

	// Fallback: If only one block found, the LLM might have combined them.
	// (In a production system, we'd add retry logic here).
	if test == "" {
		slog.Warn("⚠️  Test block missing. Retrying logic would go here.")
	}

	return code, test, nil
}

// extractBlock is a simple parser for markdown code fences.
// e.g., ```go ... ```
func extractBlock(content string, lang string, keywordHint ...string) string {
	parts := strings.Split(content, "```"+lang)
	if len(parts) < 2 {
		// Try generic block
		parts = strings.Split(content, "```")
	}

	if len(parts) < 2 {
		return ""
	}

	// If we have multiple blocks (code + test), try to identify via hint
	for _, part := range parts[1:] {
		code := strings.Split(part, "```")[0]
		if len(keywordHint) > 0 {
			if strings.Contains(strings.ToLower(code), keywordHint[0]) {
				return strings.TrimSpace(code)
			}
		} else {
			// If no hint, return the first one found
			return strings.TrimSpace(code)
		}
	}

	// Default to first valid block
	return strings.TrimSpace(strings.Split(parts[1], "```")[0])
}
