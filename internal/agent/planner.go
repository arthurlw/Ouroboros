package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"ouroboros/internal/skills"
	"strings"
	"time"
)

type Plan struct {
	Goal  string `json:"goal"`
	Steps []Step `json:"steps"`
}

type Step struct {
	ID                int    `json:"id"`
	Description       string `json:"description"`
	ToolName          string `json:"tool_name"`
	ExistingTool      bool   `json:"uses_existing_tool"`
	IsSelfImprovement bool   `json:"is_self_improvement"`
	Prompt            string `json:"prompt"`
}

type StandardPlanner struct {
	LLM LLMProvider
}

func NewPlanner(llm LLMProvider) *StandardPlanner {
	return &StandardPlanner{LLM: llm}
}

func (p *StandardPlanner) Plan(ctx context.Context, goal string, tools []skills.ToolDef) (Plan, error) {
	logger := slog.With("component", "planner", "goal", goal)
	logger.Info("🔵 STARTING PLANNING PHASE (Waiting for AI...)")

	var toolList []string
	for _, t := range tools {
		toolList = append(toolList, fmt.Sprintf("- %s: %s", t.Name, t.Description))
	}

	// NEW: Explicit instruction to reuse tool_name for multi-step evolution
	systemPrompt := fmt.Sprintf(`
You are Ouroboros, a recursive self-improving system.
Your goal: "%s"

Available Tools:
%s

INSTRUCTIONS:
1. Break the goal into atomic steps (e.g. 1. Build Core, 2. Add Optimization, 3. Add Tests).
2. CRITICAL: If Step 2 modifies Step 1, USE THE EXACT SAME 'tool_name'.
   - Bad: Step 1 'fib_calc', Step 2 'memoizer' (Creates 2 folders)
   - Good: Step 1 'fib_calc', Step 2 'fib_calc' (Refactors the same tool)
3. Return a JSON object strictly following this schema:

{
  "goal": "...",
  "steps": [
    {
      "id": 1,
      "description": "Build the basic logic",
      "tool_name": "my_tool",
      "uses_existing_tool": false,
      "is_self_improvement": false,
      "prompt": "Write a Go program that..."
    },
    {
      "id": 2,
      "description": "Add optimization",
      "tool_name": "my_tool",
      "uses_existing_tool": false,
      "is_self_improvement": false,
      "prompt": "Update the existing code to include..."
    }
  ]
}

CONSTRAINT: Return ONLY raw JSON. No markdown.
`, goal, strings.Join(toolList, "\n"))

	rawResponse, err := p.LLM.Generate(ctx, systemPrompt)
	if err != nil { return Plan{}, err }

	time.Sleep(500 * time.Millisecond)
	logger.Info("🗣️ RAW LLM RESPONSE", "content", rawResponse)

	rawResponse = cleanJSON(rawResponse)
	var plan Plan
	if err := json.Unmarshal([]byte(rawResponse), &plan); err != nil {
		logger.Error("JSON Parsing failed", "error", err)
		return Plan{}, fmt.Errorf("malformed plan: %w", err)
	}

	if len(plan.Steps) == 0 {
		return Plan{}, fmt.Errorf("AI returned empty plan")
	}

	logger.Info("🔵 PLAN GENERATED", "steps_count", len(plan.Steps))
	return plan, nil
}

func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 {
			return strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	return s
}
