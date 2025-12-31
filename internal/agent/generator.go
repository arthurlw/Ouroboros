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

func (g *Generator) GenerateCodeAndTest(ctx context.Context, requirement string) (string, string, error) {
	slog.Info("🟣 GENERATING CODE", "requirement", requirement)

	prompt := fmt.Sprintf(`
TASK: Write a GO program and a corresponding TEST SUITE.
REQUIREMENT: %s

CRITICAL INSTRUCTIONS:
1. Both the implementation and the test MUST belong to 'package main'.
2. Do NOT use 'package main_test'.
3. Do NOT try to import the code as a module.
4. The code must be self-contained.

OUTPUT FORMAT:
Provide two blocks wrapped in markdown.
Block 1: implementation.go (package main)
Block 2: implementation_test.go (package main)
`, requirement)

	raw, err := g.LLM.Generate(ctx, prompt)
	if err != nil {
		return "", "", err
	}

	code := extractBlock(raw, "go")
	test := extractBlock(raw, "go", "test")

	if code == "" {
		return "", "", fmt.Errorf("LLM failed to generate code block")
	}

	return code, test, nil
}

// extractBlock parses markdown code fences
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
		// Heuristic: If we are looking for the test, skip the block if it doesn't import "testing"
		if len(keywordHint) > 0 {
			if strings.Contains(code, keywordHint[0]) || strings.Contains(code, "testing") {
				return strings.TrimSpace(code)
			}
		} else {
			// If we want the main code, assume it's the one that DOESN'T import "testing"
			// (unless we only have one block)
			if !strings.Contains(code, "testing") {
				return strings.TrimSpace(code)
			}
		}
	}

	// Fallback: Just return the first block found if hints fail
	return strings.TrimSpace(strings.Split(parts[1], "```")[0])
}
