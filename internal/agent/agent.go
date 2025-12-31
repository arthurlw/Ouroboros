package agent

import (
	"context"
	"fmt"
	"log/slog"
	"ouroboros/internal/sandbox"
	"ouroboros/internal/skills"
)

// Agent is the central nervous system.
type Agent struct {
	Planner   *StandardPlanner
	Critic    *StandardCritic
	Generator *Generator
	Sandbox   *sandbox.Client
	Registry  *skills.Registry
	LLM       LLMProvider
	Context   context.Context
}

// EvolutionLoop runs the recursive self-improvement cycle.
func (a *Agent) EvolutionLoop(ctx context.Context, goal string) error {
	// 1. Context Injection: Load currently mastered skills
	currentTools := a.Registry.ListTools()

	// 2. Planning: Ask LLM to plan the goal
	plan, err := a.Planner.Plan(ctx, goal, currentTools)
	if err != nil {
		return fmt.Errorf("planning failed: %w", err)
	}

	for _, step := range plan.Steps {
		// Case A: Self-Improvement
		if step.IsSelfImprovement {
			if err := a.optimizeSelf(ctx, step); err != nil {
				return err
			}
			continue
		}

		// Case B: Use Existing Tool
		if step.ExistingTool {
			slog.Info("🛠️ USING EXISTING TOOL", "tool", step.ToolName)
			output, err := a.Sandbox.ExecuteTool(ctx, step.ToolName, nil)
			if err != nil {
				return err
			}
			slog.Info("Tool Output", "result", output.Stdout)
			continue
		}

		// Case C: Build New Tool
		if err := a.evolveTool(ctx, step); err != nil {
			return err
		}
	}
	return nil
}

// evolveTool handles the creation of new tools.
func (a *Agent) evolveTool(ctx context.Context, step Step) error {
	// A. Generate Code + Test
	code, test, err := a.Generator.GenerateCodeAndTest(ctx, step.Prompt)
	if err != nil {
		return err
	}

	// B. The "Ground Truth" Loop
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		// Run in Docker
		result, err := a.Sandbox.ExecuteTest(ctx, code, test)
		if err != nil {
			return fmt.Errorf("sandbox error: %w", err)
		}

		// Critique
		passed, feedback := a.Critic.Verify(ctx, result)
		if passed {
			// C. Persistence (Success)
			slog.Info("🟢 TOOL VERIFIED", "name", step.ToolName)
			return a.Registry.RegisterTool(step.ToolName, code, step.Description)
		}

		// D. Refinement (Failure)
		slog.Warn("🔴 TEST FAILED", "retry", i+1, "reason", feedback)
		code, err = a.refineCode(code, feedback)
		if err != nil {
			return err
		}
	}
	return fmt.Errorf("failed to evolve tool %s after %d attempts", step.ToolName, maxRetries)
}

func (a *Agent) refineCode(code string, feedback string) (string, error) {
	prompt := fmt.Sprintf("FIX THIS CODE.\n\nCODE:\n%s\n\nERROR:\n%s", code, feedback)
	newCode, err := a.LLM.Generate(a.Context, prompt)
	if err != nil {
		return "", err
	}
	return extractBlock(newCode, "go"), nil
}

func (a *Agent) optimizeSelf(ctx context.Context, step Step) error {
	slog.Info("⚠️ Self-optimization requested but not fully implemented in V1.")
	return nil
}
