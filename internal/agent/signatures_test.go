package agent

import (
	"strings"
	"testing"
)

// sampleSource is a small but representative main.go: imports, a const, a struct,
// an interface, a package-level var holding a function literal, a plain function,
// a method, and main().
const sampleSource = `package main

import (
	"fmt"
	"os"
)

const MaxDepth = 100

type Config struct {
	Name  string
	Depth int
}

type Runner interface {
	Run(n int) error
}

var handler = func(x int) int {
	scaled := x * 42
	return scaled
}

// Fib computes the nth Fibonacci number.
func Fib(n int) int {
	memo := map[int]int{0: 0, 1: 1}
	if v, ok := memo[n]; ok {
		return v
	}
	return Fib(n-1) + Fib(n-2)
}

func (c *Config) validate() error {
	if c.Depth > MaxDepth {
		return fmt.Errorf("depth %d exceeds %d", c.Depth, MaxDepth)
	}
	return nil
}

func main() {
	fmt.Println(os.Args)
}
`

func TestExtractSignatures_StripsBodiesKeepsSignatures(t *testing.T) {
	got := ExtractSignatures(sampleSource)

	// Signatures survive - exported, unexported, and methods alike.
	for _, want := range []string{
		"func Fib(n int) int",
		"func (c *Config) validate() error",
		"func main()",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("signature %q missing from view:\n%s", want, got)
		}
	}

	// Body statements do not.
	for _, leaked := range []string{
		"memo",
		"map[int]int",
		"Fib(n-1)",
		"exceeds",
		"os.Args",
	} {
		if strings.Contains(got, leaked) {
			t.Errorf("body fragment %q leaked into signature view:\n%s", leaked, got)
		}
	}
}

func TestExtractSignatures_KeepsTypesAndConstants(t *testing.T) {
	got := ExtractSignatures(sampleSource)

	for _, want := range []string{
		"const MaxDepth = 100",
		"type Config struct",
		"Name  string",
		"Depth int",
		"type Runner interface",
		"Run(n int) error",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("declaration %q missing from signature view:\n%s", want, got)
		}
	}
}

func TestExtractSignatures_KeepsPackageClauseAndImports(t *testing.T) {
	got := ExtractSignatures(sampleSource)

	if !strings.HasPrefix(got, "package main") {
		t.Errorf("package clause missing or not first:\n%s", got)
	}
	for _, want := range []string{`"fmt"`, `"os"`} {
		if !strings.Contains(got, want) {
			t.Errorf("import %s missing from signature view:\n%s", want, got)
		}
	}
}

// A function literal in a package-level var is still implementation. Its body
// must go, even though the declaration around it is kept.
func TestExtractSignatures_StripsFunctionLiteralBodies(t *testing.T) {
	got := ExtractSignatures(sampleSource)

	if !strings.Contains(got, "var handler = func(x int) int") {
		t.Errorf("function literal declaration missing:\n%s", got)
	}
	if strings.Contains(got, "scaled") {
		t.Errorf("function literal body leaked into signature view:\n%s", got)
	}
}

func TestExtractSignatures_UnparseableReturnsPlaceholder(t *testing.T) {
	// LLM-generated source is regularly mid-repair and syntactically invalid.
	broken := []string{
		"package main\n\nfunc Fib(n int) int {\n\tsecretConstant := 42\n", // unclosed brace
		"package main\n\nfunc broken( {\n\tsecretConstant := 42\n}\n",     // malformed params
		"this is not go at all, secretConstant = 42",
		"",
	}

	for _, src := range broken {
		got := ExtractSignatures(src)
		if got != UnparseablePlaceholder {
			t.Errorf("expected placeholder for unparseable source, got:\n%s", got)
		}
		// The point of the placeholder: failing to parse must not fall back to
		// handing the test author the full implementation.
		if strings.Contains(got, "secretConstant") {
			t.Errorf("original source leaked on the error path:\n%s", got)
		}
	}
}
