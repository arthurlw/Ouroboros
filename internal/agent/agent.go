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

	activeTools := make(map[string]bool)

	for _, step := range plan.Steps {
		if step.IsSelfImprovement && step.ToolName == "ouroboros_core" {
			continue
		}

		activeTools[step.ToolName] = true

		toolPath := filepath.Join(a.Registry.LibraryPath, step.ToolName)
		toolExists := false
		if _, err := os.Stat(toolPath); err == nil {
			toolExists = true
		}

		if toolExists && !step.ExistingTool {
			slog.Info("🛡️ REGRESSION GUARD: Tool exists. Forcing Planner to respect existing code.", "tool", step.ToolName)
		}

		if step.ExistingTool && !step.IsSelfImprovement {
			a.executeToolStep(ctx, step.ToolName, goal)
			continue
		}

		budget := ConvergenceBudget{MaxRetries: 6}
		if err := a.evolveTool(ctx, step, budget); err != nil {
			return err
		}
	}

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

func (a *Agent) runToolInternal(ctx context.Context, toolName string, goal string) (string, error) {
	toolMain := filepath.Join(a.Registry.LibraryPath, toolName, "main.go")
	codeBytes, err := os.ReadFile(toolMain)
	if err != nil { return "", err }

	args, err := a.extractArgs(goal)
	if err != nil { slog.Warn("Failed to extract args", "error", err) }

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
3. Ignore error messages from irrelevant tools.
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

	// Sanitize: Remove unused imports
	code = sanitizeImports(code)
    test = sanitizeImports(test)

	// Sanitize: Check test against implementation
	test = sanitizeTestSuite(test, code)

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
3. CRITICAL: DO NOT copy functions or structs from main.go.
4. CRITICAL: DO NOT include 'func main()'.
5. RETURN ONLY the raw Go code for main_test.go inside a markdown block.
`, code, test, feedback)

	newTest, err := a.LLM.Generate(a.Context, prompt)
	if err != nil { return "", err }

	raw := extractBlock(newTest, "go")
	return sanitizeTestSuite(raw, code), nil
}

// sanitizeTestSuite: THE SMART DEDUPLICATOR
func sanitizeTestSuite(testCode string, mainCode string) string {

	// 1. Extract function names from main.go
	// Regex matches: func FunctionName(
	reFunc := regexp.MustCompile(`func\s+([A-Za-z0-9_]+)\(`)
	mainMatches := reFunc.FindAllStringSubmatch(mainCode, -1)

	// 2. Iterate through main functions and check if they exist in test
	for _, match := range mainMatches {
		funcName := match[1]

		// Skip sanitizing Test/Benchmark functions if they somehow appear in main
		if strings.HasPrefix(funcName, "Test") || strings.HasPrefix(funcName, "Benchmark") {
			continue
		}

		// Regex to find "func FunctionName(" in test code
		reCollision := regexp.MustCompile(fmt.Sprintf(`func\s+%s\s*\(`, funcName))

		if reCollision.MatchString(testCode) {
			slog.Warn("⚠️ SANITIZER: Detected duplicate function in test file. Removing...", "func", funcName)
			// Replace "func Name(" with "// func Name( [DUPLICATE REMOVED]"
			testCode = reCollision.ReplaceAllString(testCode, fmt.Sprintf("// func %s( [DUPLICATE REMOVED]", funcName))
		}
	}

	return testCode
}

// sanitizeImports scans Go source code and removes unused imports to prevent compiler errors.
func sanitizeImports(source string) string {
	lines := strings.Split(source, "\n")

	// Helper struct to track imports
	type ImportDecl struct {
		LineIndex int
		PkgName   string
		Original  string
	}

	var imports []ImportDecl

	// Regex to parse: import "fmt" OR import foo "bar/baz"
	// Captures: 1=Alias (optional), 2=Path
	reImport := regexp.MustCompile(`^\s*(?:(\w+|_|\.)\s+)?"(.+)"`)

	inImportBlock := false

	// 1. SCAN: Find all imports
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect Import Block Start/End
		if strings.HasPrefix(trimmed, "import (") {
			inImportBlock = true
			continue
		}
		if inImportBlock && strings.HasPrefix(trimmed, ")") {
			inImportBlock = false
			continue
		}

		// Check if this line is an import
		isSingleImport := strings.HasPrefix(trimmed, "import ") && strings.Contains(trimmed, "\"")

		if inImportBlock || isSingleImport {
			cleanLine := trimmed
			if isSingleImport {
				cleanLine = strings.TrimPrefix(cleanLine, "import ")
			}

			matches := reImport.FindStringSubmatch(cleanLine)
			if len(matches) > 0 {
				alias := matches[1]
				path := matches[2]

				// Determine usage name
				pkgName := ""
				if alias != "" {
					pkgName = alias
				} else {
					// Default: last element of path (e.g. "encoding/json" -> "json")
					parts := strings.Split(path, "/")
					pkgName = parts[len(parts)-1]
				}

				// SAFETY: Preserve side-effect imports (_) and dot imports (.)
				// Dot imports are too risky to check via Regex because their functions look global.
				if alias == "_" || alias == "." {
					continue
				}

				imports = append(imports, ImportDecl{
					LineIndex: i,
					PkgName:   pkgName,
					Original:  line,
				})
			}
		}
	}

	// 2. CHECK: Build a "body" string excluding imports to check usage
	var bodyBuilder strings.Builder
	importLineIndices := make(map[int]bool)
	for _, imp := range imports {
		importLineIndices[imp.LineIndex] = true
	}

	for i, line := range lines {
		// We skip the lines we identified as imports so we don't match the import itself
		if !importLineIndices[i] {
			bodyBuilder.WriteString(line + "\n")
		}
	}
	body := bodyBuilder.String()

	// 3. FILTER: Mark lines for removal
	linesToRemove := make(map[int]bool)
	for _, imp := range imports {
		// Look for "PkgName." (e.g., "fmt.")
		// \b ensures we don't match "fmt" inside "MyfmtFunction"
		usageRe := regexp.MustCompile(fmt.Sprintf(`\b%s\.`, regexp.QuoteMeta(imp.PkgName)))

		if !usageRe.MatchString(body) {
			slog.Info("🧹 SANITIZER: Removing unused import", "package", imp.PkgName)
			linesToRemove[imp.LineIndex] = true
		}
	}

	// 4. REBUILD: Reconstruct the file
	var result []string
	for i, line := range lines {
		if !linesToRemove[i] {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

func (a *Agent) optimizeSelf(ctx context.Context, step Step) error { return nil }
