package agent

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/arthurlw/ouroboros/internal/sandbox"
	"github.com/arthurlw/ouroboros/internal/skills"
	"regexp"
	"sort"
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
			slog.Info("🛡️ REGRESSION GUARD: Tool exists. Enforcing ExistingTool=true and loading existing code.", "tool", step.ToolName)
			step.ExistingTool = true
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
	// Short-circuit if goal clearly has no arguments
	if !hasLikelyArgs(goal) {
		return []string{}, nil
	}

	prompt := fmt.Sprintf(`
GOAL: "%s"
INSTRUCTION: Extract the input arguments needed for a command-line tool.
EXAMPLE 1: "Calculate factorial of 100" -> "100"
EXAMPLE 2: "Weather in Paris" -> "Paris"
EXAMPLE 3: "Check system info" -> "none"
OUTPUT FORMAT: Return ONLY the arguments separated by spaces. If none, return "none". Do not use quotes.
`, goal)

	raw, err := a.LLM.Generate(a.Context, prompt, "")
	if err != nil { return nil, err }

	cleaned := strings.TrimSpace(raw)

	if strings.ToLower(cleaned) == "none" || cleaned == "" {
		return []string{}, nil
	}

	// Reject any output with angle brackets or braces (likely LLM artifacts)
	if strings.Contains(cleaned, "<") || strings.Contains(cleaned, "{") {
		return []string{}, nil
	}

	args := strings.Split(cleaned, " ")

	// Sanitize: reject shell metacharacters for security
	dangerousChars := []string{";", "|", "&", "`", "$", "\n", "\r"}
	for _, arg := range args {
		for _, char := range dangerousChars {
			if strings.Contains(arg, char) {
				slog.Warn("⚠️ extractArgs: Rejected argument with shell metacharacter", "arg", arg, "char", char)
				return []string{}, fmt.Errorf("argument contains dangerous character: %s", char)
			}
		}
	}

	return args, nil
}

// hasLikelyArgs checks if a goal string likely contains arguments
func hasLikelyArgs(goal string) bool {
	// Look for numbers or quoted strings that suggest arguments
	return regexp.MustCompile(`\d+|"[^"]+"|'[^']+'`).MatchString(goal)
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

	resp, err := a.LLM.Generate(a.Context, prompt, "You are Ouroboros. STRICTLY NO JSON. Plain text only.")
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

	// Sanitize: Check test against implementation (goimports handles import cleanup)
	test = sanitizeTestSuite(test, code)

	for i := 0; i < budget.MaxRetries; i++ {
		slog.Info("🔄 ITERATION", "current", i+1, "max", budget.MaxRetries)

		result, err := a.Sandbox.ExecuteTest(ctx, code, test)
		if err != nil {
			// Even on error, we may have a partial result with timeout info
			if result != nil && result.FailureKind == sandbox.FailureTimeout {
				slog.Warn("🔴 TIMEOUT - Retrying with simpler implementation")
				code, err = a.refineImplementation(code, test, "Code execution timed out. Simplify the implementation or reduce complexity.")
				if err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("sandbox fatal error: %w", err)
		}

		passed, failureKind, feedback := a.Critic.Verify(ctx, result)
		if passed {
			slog.Info("🟢 TOOL VERIFIED", "name", step.ToolName)
			return a.Registry.RegisterTool(step.ToolName, code, step.Description)
		}

		if i == budget.MaxRetries-1 {
			slog.Error("❌ CONVERGENCE FAILED: Budget Exhausted.")
			return fmt.Errorf("tool failed to converge after %d attempts", budget.MaxRetries)
		}

		slog.Warn("🔴 TEST FAILED", "kind", failureKind.String(), "reason", feedback)

		// Route refinement based on failure type
		switch failureKind {
		case sandbox.FailureBuild:
			// For build failures, check if it's in main.go or main_test.go
			if strings.Contains(feedback, "main.go") {
				code, err = a.refineImplementation(code, test, feedback)
			} else if strings.Contains(feedback, "main_test.go") {
				test, err = a.refineTestSuite(code, test, feedback)
			} else {
				code, err = a.refineImplementation(code, test, feedback)
			}
		case sandbox.FailureTimeout:
			slog.Info("🔧 SIMPLIFYING IMPLEMENTATION (Timeout)")
			code, err = a.refineImplementation(code, test, feedback)
		default:
			// For test failures and unknown errors, alternate between fixing code and tests
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

	newCode, err := a.LLM.Generate(a.Context, prompt, "You are a Senior Go Developer. Return code in markdown blocks.")
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

	newTest, err := a.LLM.Generate(a.Context, prompt, "You are a Senior Go Developer. Return code in markdown blocks.")
	if err != nil { return "", err }

	raw := extractBlock(newTest, "go")
	return sanitizeTestSuite(raw, code), nil
}

// sanitizeTestSuite: AST-based deduplicator
// Removes function declarations from test code that collide with functions in main code
func sanitizeTestSuite(testCode string, mainCode string) string {
	// Parse main.go to extract exported function names
	mainFuncs := extractFunctionNames(mainCode)
	if len(mainFuncs) == 0 {
		return testCode // Nothing to sanitize
	}

	// Parse test code
	fset := token.NewFileSet()
	testNode, err := parser.ParseFile(fset, "main_test.go", testCode, parser.ParseComments)
	if err != nil {
		slog.Warn("⚠️ SANITIZER: Failed to parse test code, skipping sanitization", "error", err)
		return testCode
	}

	// Track byte ranges to remove
	type removal struct {
		start int
		end   int
		name  string
	}
	var toRemove []removal

	// Find colliding function declarations
	for _, decl := range testNode.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			funcName := funcDecl.Name.Name

			// Skip Test/Benchmark functions
			if strings.HasPrefix(funcName, "Test") || strings.HasPrefix(funcName, "Benchmark") {
				continue
			}

			// Check for collision
			if mainFuncs[funcName] {
				slog.Warn("⚠️ SANITIZER: Detected duplicate function in test file. Removing entire declaration...", "func", funcName)
				toRemove = append(toRemove, removal{
					start: int(funcDecl.Pos() - 1), // Convert to 0-based
					end:   int(funcDecl.End() - 1),
					name:  funcName,
				})
			}
		}
	}

	// If nothing to remove, return original
	if len(toRemove) == 0 {
		return testCode
	}

	// Sort removals by position (reverse order to avoid offset issues)
	sort.Slice(toRemove, func(i, j int) bool {
		return toRemove[i].start > toRemove[j].start
	})

	// Remove the functions by rebuilding the source
	result := []byte(testCode)
	for _, rem := range toRemove {
		// Ensure bounds are valid
		if rem.start >= 0 && rem.end <= len(result) {
			result = append(result[:rem.start], result[rem.end:]...)
		}
	}

	return string(result)
}

// extractFunctionNames parses Go source and returns a set of all function names
func extractFunctionNames(code string) map[string]bool {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "main.go", code, 0)
	if err != nil {
		return nil
	}

	funcs := make(map[string]bool)
	for _, decl := range node.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			funcs[funcDecl.Name.Name] = true
		}
	}
	return funcs
}

// sanitizeImports has been removed - goimports handles import management in the sandbox

// optimizeSelf removed - implement in Phase 1 if needed
