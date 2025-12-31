package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"ouroboros/internal/skills"
	"strings"
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

// StandardPlanner is the V1 implementation of the Brain.
type StandardPlanner struct {
	LLM LLMProvider
}

func NewPlanner(llm LLMProvider) *StandardPlanner {
	return &StandardPlanner{LLM: llm}
}

func (p *StandardPlanner) Plan(ctx context.Context, goal string, tools []skills.ToolDef) (Plan, error) {
	logger := slog.With("component", "planner", "goal", goal)
	logger.Info("🔵 STARTING PLANNING PHASE")

	// Construct Context
	var toolList []string
	for _, t := range tools {
		toolList = append(toolList, fmt.Sprintf("- %s: %s", t.Name, t.Description))
	}

	systemPrompt := fmt.Sprintf(`
You are Ouroboros, a recursive self-improving system.
Your goal: "%s"

Available Tools:
%s

INSTRUCTIONS:
1. Break the goal into atomic steps.
2. If a tool exists, set 'uses_existing_tool' to true.
3. If not, set 'uses_existing_tool' to false and provide a 'prompt' to build it.
4. RETURN ONLY JSON. No markdown formatting.
`, goal, strings.Join(toolList, "\n"))

	rawResponse, err := p.LLM.Generate(ctx, systemPrompt)
	if err != nil {
		return Plan{}, err
	}

	// Clean up response if LLM adds markdown blocks
	rawResponse = cleanJSON(rawResponse)

	var plan Plan
	if err := json.Unmarshal([]byte(rawResponse), &plan); err != nil {
		logger.Error("JSON Parsing failed", "raw", rawResponse, "error", err)
		return Plan{}, fmt.Errorf("malformed plan: %w", err)
	}

	logger.Info("🔵 PLAN GENERATED", "steps_count", len(plan.Steps))
	return plan, nil
}

func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return s
}
