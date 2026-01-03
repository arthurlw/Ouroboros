package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"ouroboros/internal/sandbox"
	"ouroboros/internal/skills"
	"path/filepath"
	"regexp"
	"strings"
)

type ConvergenceBudget struct {
	MaxRetries    int
	CurrentRetry  int
}

type Agent struct {
	Planner   *StandardPlanner
	Critic    *StandardCritic
	Generator *Generator
	Sandbox   *sandbox.Client
	Registry  *skills.Registry
	LLM       LLMProvider
	Context   context.Context
}

func (a *Agent) EvolutionLoop(ctx context.Context, goal string) error {
	currentTools := a.Registry.ListTools()

	plan, err := a.Planner.Plan(ctx, goal, currentTools)
	if err != nil { return fmt.Errorf("planning failed: %w", err) }

	// NEW: Track unique tools we touched this session
	activeTools := make(map[string]bool)

	for _, step := range plan.Steps {
		if step.IsSelfImprovement && step.ToolName == "ouroboros_core" {
			continue
		}

		// Add to tracking list
		activeTools[step.ToolName] = true

		// REGRESSION GUARD
		toolPath := filepath.Join(a.Registry.LibraryPath, step.ToolName)
		toolExists := false
		if _, err := os.Stat(toolPath); err == nil {
			toolExists = true
		}

		if toolExists && !step.ExistingTool {
			slog.Info("🛡️ REGRESSION GUARD: Tool exists. Forcing Planner to respect existing code.", "tool", step.ToolName)
		}

		// "Just Run" Phase (Execution)
		if step.ExistingTool && !step.IsSelfImprovement {
			a.executeToolStep(ctx, step.ToolName, goal) // Modified signature
			continue
		}

		// BUILD / EVOLVE PHASE
		budget := ConvergenceBudget{MaxRetries: 6}
		if err := a.evolveTool(ctx, step, budget); err != nil {
			return err
		}
	}

	// FINAL CHECK: Run ALL tools we touched to ensure we catch the answer.
	// This fixes the bug where it runs the "Factorial" tool instead of the "SysReport" tool.
	slog.Info("🏁 FINAL EXECUTION: Generating Summary from all active tools...")

	var combinedOutput strings.Builder
	for toolName := range activeTools {
		output, err := a.runToolInternal(ctx, toolName, goal)
		if err == nil {
			combinedOutput.WriteString(fmt.Sprintf("--- OUTPUT FROM %s ---\n%s\n\n", toolName, output))
		}
	}

	if combinedOutput.Len() > 0 {
		response, _ := a.SynthesizeResult(goal, combinedOutput.String())
		fmt.Println("\n💬 OUROBOROS SAYS:")
		fmt.Println(response)
	}

	return nil
}

// Helper to run tool and return string (reused by executeToolStep)
func (a *Agent) runToolInternal(ctx context.Context, toolName string, goal string) (string, error) {
	toolMain := filepath.Join(a.Registry.LibraryPath, toolName, "main.go")
	codeBytes, err := os.ReadFile(toolMain)
	if err != nil { return "", err }

	args, err := a.extractArgs(goal)
	if err != nil { slog.Warn("Failed to extract args", "error", err) }

	// Quick hack: If multiple tools exist, the args might confuse one of them.
	// For V1, we accept this risk or we could ask LLM for args *per tool*.
	// We'll stick to global args for now.

	output, err := a.Sandbox.RunTool(ctx, string(codeBytes), args)
	if err != nil { return "", err }

	return output.Stdout, nil
}

func (a *Agent) executeToolStep(ctx context.Context, toolName string, goal string) {
	slog.Info("🛠️ USING TOOL", "tool", toolName)
	output, err := a.runToolInternal(ctx, toolName, goal)
	if err != nil {
		slog.Error("Tool execution failed", "error", err)
		return
	}
	slog.Info("🔍 RAW TOOL OUTPUT", "stdout", output)
}

func (a *Agent) extractArgs(goal string) ([]string, error) {
	prompt := fmt.Sprintf(`
GOAL: "%s"
INSTRUCTION: Extract the input arguments needed for a command-line tool.
EXAMPLE 1: "Calculate factorial of 100" -> "100"
EXAMPLE 2: "Weather in Paris" -> "Paris"
EXAMPLE 3: "Check system info" -> "none"
OUTPUT FORMAT: Return ONLY the arguments separated by spaces. If none, return "none". Do not use quotes.
`, goal)

	raw, err := a.LLM.Generate(a.Context, prompt)
	if err != nil { return nil, err }

	cleaned := strings.TrimSpace(raw)

	if strings.ToLower(cleaned) == "none" || cleaned == "" {
		return []string{}, nil
	}

	if strings.Contains(cleaned, "<") || strings.Contains(cleaned, "{") {
		return []string{}, nil
	}

	return strings.Split(cleaned, " "), nil
}

func (a *Agent) SynthesizeResult(goal string, rawOutput string) (string, error) {
	prompt := fmt.Sprintf(`
SYSTEM: You are Ouroboros.
USER GOAL: "%s"
ALL TOOL OUTPUTS:
%s

INSTRUCTIONS:
1. Read the outputs from all tools above.
2. Find the one that answers the user's goal.
3. Ignore error messages from irrelevant tools (e.g. ignore "Usage: <number>" if the user asked for System Info).
4. State the answer clearly.
5. STRICTLY NO JSON. Plain text only.
`, goal, rawOutput)

	resp, err := a.LLM.Generate(a.Context, prompt)
	if strings.HasPrefix(strings.TrimSpace(resp), "{") {
		return resp, err
	}
	return resp, err
}

func (a *Agent) evolveTool(ctx context.Context, step Step, budget ConvergenceBudget) error {
	var oldCode, oldTest string
	toolPath := filepath.Join(a.Registry.LibraryPath, step.ToolName)

	if _, err := os.Stat(toolPath); err == nil {
		slog.Info("📂 FOUND EXISTING TOOL. LOADING...", "tool", step.ToolName)
		c, _ := os.ReadFile(filepath.Join(toolPath, "main.go"))
		t, _ := os.ReadFile(filepath.Join(toolPath, "main_test.go"))
		oldCode = string(c)
		oldTest = string(t)
	} else {
		slog.Info("✨ CREATING NEW TOOL", "tool", step.ToolName)
	}

	code, test, err := a.Generator.GenerateCodeAndTest(ctx, step.Prompt, oldCode, oldTest)
	if err != nil { return err }

	test = sanitizeTestSuite(test)

	for i := 0; i < budget.MaxRetries; i++ {
		slog.Info("🔄 ITERATION", "current", i+1, "max", budget.MaxRetries)

		result, err := a.Sandbox.ExecuteTest(ctx, code, test)
		if err != nil { return fmt.Errorf("sandbox fatal error: %w", err) }

		passed, feedback := a.Critic.Verify(ctx, result)
		if passed {
			slog.Info("🟢 TOOL VERIFIED", "name", step.ToolName)
			return a.Registry.RegisterTool(step.ToolName, code, step.Description)
		}

		if i == budget.MaxRetries-1 {
			slog.Error("❌ CONVERGENCE FAILED: Budget Exhausted.")
			return fmt.Errorf("tool failed to converge after %d attempts", budget.MaxRetries)
		}

		slog.Warn("🔴 TEST FAILED", "reason", feedback)

		if strings.Contains(feedback, "[build failed]") {
			if strings.Contains(feedback, "main.go") {
				code, err = a.refineImplementation(code, test, feedback)
			} else if strings.Contains(feedback, "main_test.go") {
				test, err = a.refineTestSuite(code, test, feedback)
			} else {
				code, err = a.refineImplementation(code, test, feedback)
			}
		} else {
			if i%2 == 0 {
				slog.Info("🔧 REFINING IMPLEMENTATION (Aligning Code to Test)")
				code, err = a.refineImplementation(code, test, feedback)
			} else {
				slog.Info("🔧 REFINING TEST SUITE (Aligning Test to Code)")
				test, err = a.refineTestSuite(code, test, feedback)
			}
		}

		if err != nil { return err }
	}
	return nil
}

func (a *Agent) refineImplementation(code string, test string, feedback string) (string, error) {
	prompt := fmt.Sprintf(`
You are a Senior Go Developer.
TASK: Fix the implementation file (main.go).

CONTEXT - IMPLEMENTATION (main.go):
%s

CONTEXT - TEST SUITE (main_test.go) [READ ONLY]:
%s

ERROR LOG:
%s

INSTRUCTIONS:
1. Fix the logic error in main.go.
2. Ensure 'package main'.
3. DO NOT include test functions (TestXXX).
4. RETURN ONLY the raw Go code for main.go inside a markdown block.
`, code, test, feedback)

	newCode, err := a.LLM.Generate(a.Context, prompt)
	if err != nil { return "", err }
	return extractBlock(newCode, "go"), nil
}

func (a *Agent) refineTestSuite(code string, test string, feedback string) (string, error) {
	prompt := fmt.Sprintf(`
You are a Senior Go Developer.
TASK: Fix the test suite (main_test.go).

CONTEXT - IMPLEMENTATION (main.go) [READ ONLY - DO NOT COPY]:
%s

CONTEXT - TEST SUITE (main_test.go):
%s

ERROR LOG:
%s

INSTRUCTIONS:
1. Fix the test logic.
2. Ensure 'package main'.
3. CRITICAL: DO NOT copy functions or structs from main.go. They are already in the package.
4. CRITICAL: DO NOT include 'func main()'.
5. RETURN ONLY the raw Go code for main_test.go inside a markdown block.
`, code, test, feedback)

	newTest, err := a.LLM.Generate(a.Context, prompt)
	if err != nil { return "", err }

	raw := extractBlock(newTest, "go")
	return sanitizeTestSuite(raw), nil
}

func sanitizeTestSuite(content string) string {
	if strings.Contains(content, "func main()") {
		re := regexp.MustCompile(`func main\(\)\s*\{`)
		content = re.ReplaceAllString(content, "// func main() removed by sanitizer {")
	}
	return content
}

func (a *Agent) optimizeSelf(ctx context.Context, step Step) error { return nil }
