package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"ouroboros/internal/sandbox"
	"ouroboros/internal/skills"
	"path/filepath"
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

	for _, step := range plan.Steps {
		if step.IsSelfImprovement {
			if err := a.optimizeSelf(ctx, step); err != nil { return err }
			continue
		}

		if step.ExistingTool {
			slog.Info("🛠️ USING EXISTING TOOL", "tool", step.ToolName)
			_, err := a.Sandbox.ExecuteTool(ctx, step.ToolName, nil)
			if err != nil { return err }
			continue
		}

		budget := ConvergenceBudget{MaxRetries: 6}
		if err := a.evolveTool(ctx, step, budget); err != nil {
			return err
		}
	}
	return nil
}

func (a *Agent) evolveTool(ctx context.Context, step Step, budget ConvergenceBudget) error {
	// 1. CHECK FOR EXISTING CODE (The "In-Place" Fix)
	var oldCode, oldTest string

	// We look in the verified library path for this tool name
	toolPath := filepath.Join(a.Registry.LibraryPath, step.ToolName)
	if _, err := os.Stat(toolPath); err == nil {
		slog.Info("📂 FOUND EXISTING TOOL VERSION. LOADING...", "tool", step.ToolName)
		// Best effort load
		c, _ := os.ReadFile(filepath.Join(toolPath, "main.go"))
		t, _ := os.ReadFile(filepath.Join(toolPath, "main_test.go"))
		oldCode = string(c)
		oldTest = string(t)
	}

	// 2. GENERATE (Passing old code if it exists)
	code, test, err := a.Generator.GenerateCodeAndTest(ctx, step.Prompt, oldCode, oldTest)
	if err != nil { return err }

	// 3. CONVERGENCE LOOP
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
				code, err = a.refineCode(code, test, feedback, "main")
			} else if strings.Contains(feedback, "main_test.go") {
				test, err = a.refineCode(code, test, feedback, "test")
			} else {
				code, err = a.refineCode(code, test, feedback, "main")
			}
		} else {
			if i%2 == 0 {
				slog.Info("🔧 REFINING IMPLEMENTATION (Aligning Code to Test)")
				code, err = a.refineCode(code, test, feedback, "main")
			} else {
				slog.Info("🔧 REFINING TEST SUITE (Aligning Test to Code)")
				test, err = a.refineCode(code, test, feedback, "test")
			}
		}

		if err != nil { return err }
	}
	return nil
}

func (a *Agent) refineCode(code string, test string, feedback string, target string) (string, error) {
	var instructions string
	if target == "main" {
		instructions = `
1. ANALYZE the test failure.
2. Fix the logic in main.go.
3. Ensure 'package main'.
4. DO NOT include test functions.`
	} else {
		instructions = `
1. ANALYZE the test data vs requirements.
2. Fix the assertions in main_test.go.
3. Ensure 'package main'.
4. DO NOT redeclare structs defined in main.go.`
	}

	prompt := fmt.Sprintf(`
You are a Senior Go Developer fixing a %s file.

CONTEXT - IMPLEMENTATION (main.go):
%s

CONTEXT - TEST SUITE (main_test.go):
%s

ERROR LOG:
%s

INSTRUCTIONS:
%s
5. OUTPUT FORMAT: Return ONLY the raw Go code for the **%s** file inside a markdown block.
`, target, code, test, feedback, instructions, target)

	newCode, err := a.LLM.Generate(a.Context, prompt)
	if err != nil { return "", err }

	extracted := extractBlock(newCode, "go")
	if extracted == "" {
		if strings.Contains(newCode, "package main") {
			return newCode, nil
		}
		return newCode, nil
	}
	return extracted, nil
}

func (a *Agent) optimizeSelf(ctx context.Context, step Step) error {
	slog.Info("⚠️ Self-optimization requested but not fully implemented in V1.")
	return nil
}
