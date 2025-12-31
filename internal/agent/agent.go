package agent

import (
	"context"
	"fmt"
	"os"
	"ouroboros/internal/sandbox"
	"ouroboros/internal/skills"
)

// IPlanner defines the logic for breaking down goals.
// Ouroboros can rewrite this interface's implementation to get smarter.
type IPlanner interface {
	Plan(ctx context.Context, goal string, availableTools []skills.ToolDef) (Plan, error)
}

// ICritic defines how code is judged.
// Ouroboros can improve its own test-writing logic by swapping this.
type ICritic interface {
	Verify(ctx context.Context, result *sandbox.Result) (bool, string)
}

// Agent is the kernel of the system.
type Agent struct {
	Planner  IPlanner        // The current "Strategy" module
	Critic   ICritic         // The current "Quality Control" module
	Sandbox  *sandbox.Client // The "Gym"
	Registry *skills.Registry // The "Memory"
}

// EvolutionLoop is the main lifecycle.
func (a *Agent) EvolutionLoop(ctx context.Context, goal string) error {
	// 1. Context Injection: "What can I do right now?"
	tools := a.Registry.ListTools()

	// 2. Plan: Decompose the goal using the current Planner strategy
	plan, err := a.Planner.Plan(ctx, goal, tools)
	if err != nil {
		return fmt.Errorf("planning failed: %w", err)
	}

	for _, step := range plan.Steps {
		if step.IsSelfImprovement {
			// Special Path: Modifying Ouroboros itself
			if err := a.optimizeSelf(ctx, step); err != nil {
				return err
			}
			continue
		}

		// Standard Path: Building a new tool
		if err := a.evolveTool(ctx, step); err != nil {
			return err
		}
	}
	return nil
}

// evolveTool handles the creation of external tools (e.g., "Write a CSV Parser").
func (a *Agent) evolveTool(ctx context.Context, step Step) error {
	// A. Generate Code + Test (The "Candidate")
	// In reality, you'd call the LLM here.
	code, test, err := a.generateCodeAndTest(step.Prompt)
	if err != nil { return err }

	// B. The "Ground Truth" Loop
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		// Run in Docker
		result, err := a.Sandbox.ExecuteTest(ctx, code, test)
		if err != nil { return err }

		// Critique
		passed, feedback := a.Critic.Verify(ctx, result)
		if passed {
			// C. MCP Persistence (Success)
			return a.Registry.RegisterTool(step.ToolName, code, step.Description)
		}

		// D. Failure-Driven Refinement
		// Agent attempts to fix the code using the error logs
		code, err = a.refineCode(code, feedback)
		if err != nil { return err }
	}
	return fmt.Errorf("failed to evolve tool %s after %d attempts", step.ToolName, maxRetries)
}

// optimizeSelf handles Core Directive #5.
// It reads its own source, generates a patch, benchmarks it, and proposes a swap.
func (a *Agent) optimizeSelf(ctx context.Context, step Step) error {
	// 1. Read Own Source
	currentSource, _ := os.ReadFile("internal/agent/planner.go")

	// 2. Generate Optimization (e.g., "Make the planner use a Tree of Thoughts")
	// candidateSource := LLM.Generate(currentSource, "Optimize for breadth-first search")

	// 3. Benchmark (Strict validation)
	// We run a special benchmark in the sandbox to prove v2 is faster/better than v1.
	// result := a.Sandbox.RunBenchmark(candidateSource)

	// 4. If result.Score > currentScore:
	// Write "planner_v2.go" to disk so the user can recompile.
	return nil
}
