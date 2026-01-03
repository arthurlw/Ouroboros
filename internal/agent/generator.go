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

func (g *Generator) GenerateCodeAndTest(ctx context.Context, requirement string, existingCode string, existingTest string) (string, string, error) {
	slog.Info("🟣 GENERATING CODE", "requirement", requirement, "is_refactor", existingCode != "")

	var prompt string
	if existingCode == "" {
		// MODE A: NEW TOOL
		prompt = fmt.Sprintf(`
TASK: Write a GO program and TEST SUITE.
REQUIREMENT: %s

INSTRUCTIONS:
1. Both files must start with 'package main'.
2. The code must be self-contained.
3. CRITICAL: The main() function should accept a command-line argument for input.
4. Check imports.

OUTPUT FORMAT:
Block 1: implementation (markdown 'go')
Block 2: test (markdown 'go')
`, requirement)
	} else {
		// MODE B: REFACTOR (The "Anti-Downgrade" Fix)
		prompt = fmt.Sprintf(`
TASK: Refactor/Update this GO program.
NEW REQUIREMENT: %s

EXISTING IMPLEMENTATION:
%s

EXISTING TEST:
%s

CRITICAL INSTRUCTIONS:
1. Compare the NEW REQUIREMENT with the EXISTING IMPLEMENTATION.
2. IF the Existing Implementation is ALREADY better (e.g. uses math/big, handles complex inputs) -> KEEP IT. Do NOT downgrade it to a basic version just because the requirement is simple.
3. ONLY modify the code to FIX bugs or ADD features.
4. Ensure 'package main'.
5. DO NOT copy implementation logic into the test file.

OUTPUT FORMAT:
Block 1: Updated implementation (markdown 'go')
Block 2: Updated test (markdown 'go')
`, requirement, existingCode, existingTest)
	}

	raw, err := g.LLM.Generate(ctx, prompt)
	if err != nil { return "", "", err }

	code := extractBlock(raw, "go")
	test := extractBlock(raw, "go", "test")

	if test == "" && existingTest != "" {
		test = existingTest
	}

	if code == "" {
		return "", "", fmt.Errorf("LLM failed to generate code block")
	}

	return code, test, nil
}

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
