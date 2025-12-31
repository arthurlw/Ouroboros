package agent

// Add this method to the Agent struct in agent.go
func (a *Agent) generateCodeAndTest(prompt string) (string, string, error) {
	// We assume Agent holds a reference to a Generator
	// You need to update the Agent struct in agent.go to include:
	// Generator *Generator
	return a.Generator.GenerateCodeAndTest(a.Context, prompt)
}

// Add this method to Agent struct in agent.go
func (a *Agent) refineCode(code string, feedback string) (string, error) {
	// Simple loopback: Ask LLM to fix code based on stderr
	prompt := fmt.Sprintf("FIX THIS CODE.\n\nCODE:\n%s\n\nERROR:\n%s", code, feedback)
	newCode, err := a.LLM.Generate(a.Context, prompt)
	if err != nil {
		return "", err
	}
	return extractBlock(newCode, "go"), nil
}