package agent

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CodeSkeleton represents the structure AND dependencies of a file.
type CodeSkeleton struct {
	Path         string
	Package      string
	Interfaces   []string
	Structs      []string
	Functions    []string            // Signatures only
	Dependencies map[string][]string // FuncName -> List of functions it calls
}

// SummarizeAgent scans the /internal directory and returns a high-level map with dependencies.
func SummarizeAgent(rootPath string) (string, error) {
	var summary strings.Builder
	summary.WriteString("## SYSTEM ARCHITECTURE & CALL GRAPH\n\n")

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		skel, err := parseFile(path)
		if err != nil {
			return nil // Skip unparseable files
		}

		summary.WriteString(fmt.Sprintf("### FILE: %s (Package: %s)\n", skel.Path, skel.Package))

		if len(skel.Interfaces) > 0 {
			summary.WriteString("**Interfaces:**\n")
			for _, iface := range skel.Interfaces {
				summary.WriteString(fmt.Sprintf("- %s\n", iface))
			}
		}

		if len(skel.Structs) > 0 {
			summary.WriteString("**Structs:**\n")
			for _, struc := range skel.Structs {
				summary.WriteString(fmt.Sprintf("- %s\n", struc))
			}
		}

		if len(skel.Functions) > 0 {
			summary.WriteString("**Functions & Dependencies:**\n")
			// Zip signatures with dependencies
			// Note: This relies on order, but for a summary, exact mapping via name is better.
			// For simplicity here, we list the signature and then its calls.
			for _, fnSig := range skel.Functions {
				// Extract function name from signature for lookup (simplified heuristic)
				// Signature: "func (a *Agent) Run(...) ..." -> Name: "Run"
				funcName := extractNameFromSig(fnSig)
				calls := skel.Dependencies[funcName]

				summary.WriteString(fmt.Sprintf("- %s\n", fnSig))
				if len(calls) > 0 {
					// Deduplicate and sort calls for clean output
					uniqueCalls := uniqueStrings(calls)
					if len(uniqueCalls) > 0 {
						summary.WriteString(fmt.Sprintf("  ↳ Calls: [%s]\n", strings.Join(uniqueCalls, ", ")))
					}
				}
			}
		}
		summary.WriteString("\n")
		return nil
	})

	return summary.String(), err
}

func parseFile(path string) (CodeSkeleton, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return CodeSkeleton{}, err
	}

	skel := CodeSkeleton{
		Path:         path,
		Package:      node.Name.Name,
		Dependencies: make(map[string][]string),
	}

	ast.Inspect(node, func(n ast.Node) bool {
		switch t := n.(type) {
		case *ast.TypeSpec:
			if _, ok := t.Type.(*ast.InterfaceType); ok {
				skel.Interfaces = append(skel.Interfaces, t.Name.Name)
			} else if _, ok := t.Type.(*ast.StructType); ok {
				skel.Structs = append(skel.Structs, t.Name.Name)
			}
		case *ast.FuncDecl:
			funcName := t.Name.Name

			// 1. Capture Signature
			var buf bytes.Buffer
			// Create a copy without body for printing signature
			tempFn := *t
			tempFn.Body = nil
			printer.Fprint(&buf, fset, &tempFn)
			skel.Functions = append(skel.Functions, buf.String())

			// 2. Capture Dependencies (Walk the BODY)
			if t.Body != nil {
				ast.Inspect(t.Body, func(child ast.Node) bool {
					if call, ok := child.(*ast.CallExpr); ok {
						calledName := ""
						switch fun := call.Fun.(type) {
						case *ast.Ident:
							// Local call: "runToolInternal(...)"
							calledName = fun.Name
						case *ast.SelectorExpr:
							// Method/Package call: "a.Sandbox.RunTool(...)" or "fmt.Println(...)"
							// We want the Selector (Method Name)
							calledName = fun.Sel.Name
							// Optional: Capture "X.Sel" (e.g. "Sandbox.RunTool")
							if xIdent, ok := fun.X.(*ast.Ident); ok {
								calledName = xIdent.Name + "." + fun.Sel.Name
							}
						}
						if calledName != "" {
							skel.Dependencies[funcName] = append(skel.Dependencies[funcName], calledName)
						}
					}
					return true
				})
			}
		}
		return true
	})

	return skel, nil
}

// Helpers

func extractNameFromSig(sig string) string {
	// Heuristic: Find the identifier before the first '('
	// "func (a *Agent) Run(" -> "Run"
	// "func Run(" -> "Run"

	// Remove "func " prefix
	s := strings.TrimPrefix(sig, "func ")

	// Remove receiver "(...)" if present
	if strings.HasPrefix(s, "(") {
		endReceiver := strings.Index(s, ")")
		if endReceiver != -1 {
			s = strings.TrimSpace(s[endReceiver+1:])
		}
	}

	// Now s starts with FunctionName
	idx := strings.Index(s, "(")
	if idx == -1 { return "" }
	return strings.TrimSpace(s[:idx])
}

func uniqueStrings(input []string) []string {
	u := make(map[string]bool)
	var res []string
	for _, val := range input {
		if !u[val] {
			u[val] = true
			res = append(res, val)
		}
	}
	sort.Strings(res)
	return res
}
