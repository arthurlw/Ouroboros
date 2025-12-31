package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

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
1. Both files must start with 'package main'.
2. CHECK YOUR IMPORTS. Go compiler is strict.
   - Do NOT import "fmt" or "strings" if you don't use them.
   - Do NOT use "strings." functions without importing "strings".
3. The code must be self-contained.

OUTPUT FORMAT:
Block 1: implementation (markdown 'go')
Block 2: test (markdown 'go')
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
			if strings.Contains(code, keywordHint[0]) || strings.Contains(code, "testing") {
				return strings.TrimSpace(code)
			}
		} else {
			if !strings.Contains(code, "testing") {
				return strings.TrimSpace(code)
			}
		}
	}
	return strings.TrimSpace(strings.Split(parts[1], "```")[0])
}
