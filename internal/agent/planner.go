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

// Plan represents the roadmap the agent generates.
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

	// 1. Construct Context
	var toolList []string
	for _, t := range tools {
		toolList = append(toolList, fmt.Sprintf("- %s: %s", t.Name, t.Description))
	}

	// 2. Strict Prompt
	systemPrompt := fmt.Sprintf(`
You are Ouroboros, a recursive self-improving system.
Your goal: "%s"

Available Tools:
%s

INSTRUCTIONS:
1. Break the goal into atomic steps.
2. Return a JSON object strictly following this schema:

{
  "goal": "The user's goal",
  "steps": [
    {
      "id": 1,
      "description": "Generate the code",
      "tool_name": "password_generator",
      "uses_existing_tool": false,
      "is_self_improvement": false,
      "prompt": "Write a Go program that generates a random secure password..."
    }
  ]
}

CONSTRAINT: Return ONLY raw JSON. No markdown.
`, goal, strings.Join(toolList, "\n"))

	// 3. Call LLM
	rawResponse, err := p.LLM.Generate(ctx, systemPrompt)
	if err != nil {
		return Plan{}, err
	}

	// SLOW DOWN: Artificial delay so you can see the logs in terminal
	time.Sleep(500 * time.Millisecond)

	// 4. Debug Logging (CRITICAL: This tells us what the AI actually said)
	logger.Info("🗣️ RAW LLM RESPONSE", "content", rawResponse)

	// 5. Clean & Parse
	rawResponse = cleanJSON(rawResponse)
	var plan Plan
	if err := json.Unmarshal([]byte(rawResponse), &plan); err != nil {
		logger.Error("JSON Parsing failed", "error", err)
		return Plan{}, fmt.Errorf("malformed plan: %w", err)
	}

	// 6. Verification
	if len(plan.Steps) == 0 {
		logger.Error("⚠️  PLAN WAS EMPTY! The AI returned valid JSON but 0 steps.")
		return Plan{}, fmt.Errorf("AI returned empty plan")
	}

	logger.Info("🔵 PLAN GENERATED", "steps_count", len(plan.Steps))
	return plan, nil
}

func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	// Remove markdown code fences if present
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 {
			return strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	return s
}
