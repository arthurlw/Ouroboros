package agent

import (
	"context"
	"strings"
	"testing"
)

// capturingLLM records the prompt it is handed so a test can assert on what
// actually entered the context window.
type capturingLLM struct {
	prompt string
	reply  string
}

func (c *capturingLLM) Generate(ctx context.Context, prompt string) (string, error) {
	c.prompt = prompt
	return c.reply, nil
}

const visibilityImpl = `package main

import "fmt"

// Discount applies the tiered discount.
func Discount(total float64) float64 {
	rate := 0.05
	if total > 1000 {
		rate = 0.20
	}
	return total - total*rate
}

func main() {
	fmt.Println(Discount(1200))
}
`

const visibilityTest = `package main

import "testing"

func TestDiscountHighTier(t *testing.T) {
	if got := Discount(1200); got != 960 {
		t.Errorf("got %v", got)
	}
}
`

const visibilitySpec = "Apply a 5% discount below 1000 and 20% at or above 1000."
const visibilityFeedback = "TEST FAILURE:\n--- FAIL: TestDiscountHighTier (0.00s)\n\tmain_test.go:5: got 1140"

func newFakeAgent(reply string) (*Agent, *capturingLLM) {
	llm := &capturingLLM{reply: reply}
	return &Agent{LLM: llm, Context: context.Background()}, llm
}

// The implementer must never see main_test.go.
func TestRefineImplementation_DoesNotSeeTestSource(t *testing.T) {
	a, llm := newFakeAgent("```go\npackage main\n```")

	if _, err := a.refineImplementation(visibilityImpl, visibilitySpec, visibilityFeedback); err != nil {
		t.Fatalf("refineImplementation: %v", err)
	}

	for _, leaked := range []string{"func TestDiscountHighTier", "t.Errorf", `import "testing"`} {
		if strings.Contains(llm.prompt, leaked) {
			t.Errorf("test source fragment %q reached the implementer prompt", leaked)
		}
	}

	// It does get its own file, the spec, and the failure feedback.
	for _, want := range []string{"rate = 0.20", visibilitySpec, "TestDiscountHighTier (0.00s)"} {
		if !strings.Contains(llm.prompt, want) {
			t.Errorf("implementer prompt missing %q", want)
		}
	}
}

// The test author must never see the bodies of main.go - only its signatures.
func TestRefineTestSuite_SeesSignaturesNotBodies(t *testing.T) {
	a, llm := newFakeAgent("```go\npackage main\n\nimport \"testing\"\n```")

	if _, err := a.refineTestSuite(visibilityImpl, visibilityTest, visibilitySpec, visibilityFeedback); err != nil {
		t.Fatalf("refineTestSuite: %v", err)
	}

	for _, leaked := range []string{"rate := 0.05", "rate = 0.20", "total > 1000", "total*rate"} {
		if strings.Contains(llm.prompt, leaked) {
			t.Errorf("implementation body fragment %q reached the test-author prompt", leaked)
		}
	}

	for _, want := range []string{
		"func Discount(total float64) float64", // signature survives
		"func TestDiscountHighTier",            // its own file
		visibilitySpec,
		"main_test.go:5: got 1140",
	} {
		if !strings.Contains(llm.prompt, want) {
			t.Errorf("test-author prompt missing %q", want)
		}
	}
}

// When main.go does not parse, the test author gets the placeholder rather than
// the full source.
func TestRefineTestSuite_UnparseableImplementationIsNotLeaked(t *testing.T) {
	a, llm := newFakeAgent("```go\npackage main\n\nimport \"testing\"\n```")

	broken := "package main\n\nfunc Discount(total float64) float64 {\n\trate := 0.05\n"

	if _, err := a.refineTestSuite(broken, visibilityTest, visibilitySpec, visibilityFeedback); err != nil {
		t.Fatalf("refineTestSuite: %v", err)
	}

	if strings.Contains(llm.prompt, "rate := 0.05") {
		t.Error("unparseable implementation was leaked into the test-author prompt")
	}
	if !strings.Contains(llm.prompt, UnparseablePlaceholder) {
		t.Errorf("expected the unparseable placeholder in the prompt:\n%s", llm.prompt)
	}
}
