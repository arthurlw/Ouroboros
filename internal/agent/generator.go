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

OUTPUT FORMAT:
You must provide two distinct blocks of code wrapped in markdown.
Block 1: The implementation (package main)
Block 2: The test (package main_test)

CONSTRAINT:
- Use standard library only where possible.
- The code must be self-contained in one file logic.
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
		if len(keywordHint) > 0 {
			if strings.Contains(strings.ToLower(code), keywordHint[0]) {
				return strings.TrimSpace(code)
			}
		} else {
			return strings.TrimSpace(code)
		}
	}

	// Default to first block found
	return strings.TrimSpace(strings.Split(parts[1], "```")[0])
}
