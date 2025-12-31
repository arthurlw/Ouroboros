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
	"strings"
)

// CodeSkeleton represents the "Interface Definition" of the system.
type CodeSkeleton struct {
	Path       string
	Package    string
	Interfaces []string
	Structs    []string
	Functions  []string // Signatures only
}

// SummarizeAgent scans the /internal directory and returns a high-level map.
func SummarizeAgent(rootPath string) (string, error) {
	var summary strings.Builder
	summary.WriteString("## SYSTEM ARCHITECTURE SKELETON\n\n")

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		skel, err := parseFile(path)
		if err != nil {
			return nil // Skip unparseable files
		}

		summary.WriteString(fmt.Sprintf("### FILE: %s (Package: %s)\n", skel.Path, skel.Package))
		for _, iface := range skel.Interfaces {
			summary.WriteString(fmt.Sprintf("- INTERFACE: %s\n", iface))
		}
		for _, struc := range skel.Structs {
			summary.WriteString(fmt.Sprintf("- STRUCT: %s\n", struc))
		}
		for _, fn := range skel.Functions {
			summary.WriteString(fmt.Sprintf("- FUNC: %s\n", fn))
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
		Path:    path,
		Package: node.Name.Name,
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
			// Render only the signature (Name + Params + Results), strip the body
			var buf bytes.Buffer
			// Create a copy without body for printing
			tempFn := *t
			tempFn.Body = nil
			printer.Fprint(&buf, fset, &tempFn)
			skel.Functions = append(skel.Functions, buf.String())
		}
		return true
	})

	return skel, nil
}
