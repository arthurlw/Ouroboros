package agent

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
)

// UnparseablePlaceholder is returned by ExtractSignatures when the source cannot
// be parsed. It is deliberately NOT a fallback to the original source: the whole
// point of the signature view is that the test author never sees implementation
// bodies, so an unparseable file must degrade to "nothing" rather than "everything".
const UnparseablePlaceholder = "// [implementation unparseable this iteration]"

// ExtractSignatures returns an interface-only view of a Go source file: the
// package clause, the imports, every type/struct/interface/const/var declaration,
// and every function and method signature - with all function bodies removed.
//
// It is the only form in which the test-author role is allowed to see main.go.
// The test author needs the interface to write a test suite that compiles; it
// must not see the logic, or it will write tests that mirror the implementation's
// mistakes instead of checking the spec.
//
// Parsing is done with go/parser, not regex, so the result is exact rather than
// best-effort. Comments are discarded along with the bodies: a comment inside a
// function body is implementation detail, and go/printer positions comments
// independently of the declarations they were attached to, so keeping any of them
// risks re-emitting body text.
//
// If the source does not parse - which happens routinely, since it is LLM-generated
// and may be mid-repair - UnparseablePlaceholder is returned.
func ExtractSignatures(source string) string {
	fset := token.NewFileSet()

	// parser.SkipObjectResolution: we only print, we never resolve identifiers.
	// Comments are not requested, so none are carried into the output.
	file, err := parser.ParseFile(fset, "main.go", source, parser.SkipObjectResolution)
	if err != nil {
		return UnparseablePlaceholder
	}

	stripBodies(file)

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return UnparseablePlaceholder
	}

	out := strings.TrimSpace(buf.String())
	if out == "" {
		return UnparseablePlaceholder
	}
	return out
}

// stripBodies removes every executable statement from the file in place.
//
// Top-level functions and methods lose their body entirely (go/printer emits a
// bare signature for a *ast.FuncDecl whose Body is nil). Function literals -
// e.g. "var handler = func(x int) int { ... }" - keep an empty block instead, so
// the surrounding declaration still prints as valid Go.
func stripBodies(file *ast.File) {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			fn.Body = nil
			continue
		}
		// A GenDecl (const/var/type) can still hide statements inside a
		// function literal used as an initializer.
		ast.Inspect(decl, func(n ast.Node) bool {
			if lit, ok := n.(*ast.FuncLit); ok {
				lit.Body = &ast.BlockStmt{}
				return false
			}
			return true
		})
	}
}
