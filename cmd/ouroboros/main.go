// EvolutionLoop runs the recursive self-improvement cycle.
func (a *Agent) EvolutionLoop(goal string) error {
	// 1. Context Injection: Load currently mastered skills into the LLM's context
	currentTools := a.Registry.GetActiveMCPTools()

	// 2. Planning: Ask LLM to plan the goal using *only* currentTools or request a *new* tool.
	plan, err := a.Plan(goal, currentTools)
	if err != nil {
		return err
	}

	for _, step := range plan.Steps {
		// Case A: We have the tool. Execute it.
		if step.ToolExists {
			result, _ := a.ExecuteTool(step.ToolName, step.Args)
			a.Evaluate(result)
			continue
		}

		// Case B: Tool missing. We must evolve.
		// 3. Generation: Agent writes the implementation AND a test suite.
		code, testSuite := a.GenerateTool(step.Requirement)

		// 4. Critique (The "Ground Truth" Gatekeeper): Run in Docker.
		sandboxResult, err := a.Sandbox.RunTest(code, testSuite)

		if err != nil || !sandboxResult.Passed {
			// Recursion: The agent reads the error and retries generation
			// (You would add a retry limit here)
			a.RefineTool(code, sandboxResult.Logs)
		} else {
			// 5. MCP Persistence: Promotion to Long-term Memory.
			// Save the code to disk and update the MCP definition.
			err := a.Registry.RegisterSkill(step.NewToolName, code, step.Description)
			if err != nil {
				return err // Handle persistence failure
			}

			// 6. Dynamic Re-injection: The loop restarts, but now 'currentTools'
			// includes the new tool we just built.
		}
	}

	return nil
}
