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
	ToolName          string `json:"tool_name"`           // e.g., "csv_parser"
	ExistingTool      bool   `json:"uses_existing_tool"`  // True = call it; False = build it
	IsSelfImprovement bool   `json:"is_self_improvement"` // The "Ouroboros" flag
	Prompt            string `json:"prompt"`              // Instructions for the generator
}

// StandardPlanner is the V1 implementation of the Brain.
type StandardPlanner struct {
	LLM LLMProvider
}

func NewPlanner(llm LLMProvider) *StandardPlanner {
	return &StandardPlanner{LLM: llm}
}

func (p *StandardPlanner) Plan(ctx context.Context, goal string, tools []skills.ToolDef) (Plan, error) {
	// OBSERVABILITY: Start the trace
	logger := slog.With("component", "planner", "goal", goal)
	logger.Info("🔵 STARTING PLANNING PHASE")

	// 1. Construct the Context
	// We must feed the agent its own current capabilities.
	var toolList []string
	for _, t := range tools {
		toolList = append(toolList, fmt.Sprintf("- %s: %s", t.Name, t.Description))
	}

	systemPrompt := fmt.Sprintf(`
You are Ouroboros, a recursive self-improving system.
Your goal: "%s"

Available Tools (Do not rebuild these unless necessary):
%s

INSTRUCTIONS:
1. Break the goal into atomic steps.
2. For each step, decide if you can use an EXISTING tool or must BUILD a new one.
3. If the goal implies modifying your own internal architecture, mark 'is_self_improvement' as true.
4. RETURN ONLY JSON. No markdown. No prologue.
`, goal, strings.Join(toolList, "\n"))

	// OBSERVABILITY: Log what we are asking (crucial for prompt debugging)
	logger.Debug("Sending prompt to LLM", "prompt_len", len(systemPrompt))

	// 2. Call LLM
	// (Assumes LLM provider handles the actual HTTP request)
	rawResponse, err := p.LLM.Generate(ctx, systemPrompt)
	if err != nil {
		logger.Error("Planning failed at LLM layer", "error", err)
		return Plan{}, err
	}

	// OBSERVABILITY: Log the raw thought before parsing
	logger.Debug("Raw LLM Response", "response", rawResponse)

	// 3. Parse JSON
	var plan Plan
	if err := json.Unmarshal([]byte(rawResponse), &plan); err != nil {
		// CRITICAL EVALUATION: This is where agents usually die.
		// We log the failure explicitly to help you tweak the system prompt.
		logger.Error("JSON Parsing failed. LLM hallucinated invalid format.",
			"raw", rawResponse,
			"error", err,
		)
		return Plan{}, fmt.Errorf("malformed plan: %w", err)
	}

	// OBSERVABILITY: Success
	logger.Info("🔵 PLAN GENERATED", "steps_count", len(plan.Steps))
	for _, step := range plan.Steps {
		logger.Info("Step Detail",
			"id", step.ID,
			"tool", step.ToolName,
			"action", map[bool]string{true: "USE", false: "BUILD"}[step.ExistingTool],
		)
	}

	return plan, nil
}
