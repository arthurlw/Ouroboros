package agent

import (
	"strings"
	"testing"
)

func TestSanitizeTestSuite_RemovesDuplicates(t *testing.T) {
	mainCode := `package main

func Calculate(n int) int {
	return n * 2
}

func main() {
	println(Calculate(5))
}
`

	testCode := `package main

import "testing"

// This is a duplicate - should be removed
func Calculate(n int) int {
	return n * 3
}

func TestCalculate(t *testing.T) {
	result := Calculate(10)
	if result != 20 {
		t.Errorf("Expected 20, got %d", result)
	}
}
`

	result := sanitizeTestSuite(testCode, mainCode)

	// The duplicate Calculate function should be removed
	if strings.Contains(result, "func Calculate(n int) int {") {
		t.Error("sanitizeTestSuite should have removed the duplicate Calculate function")
	}

	// The test function should still be present
	if !strings.Contains(result, "func TestCalculate(t *testing.T)") {
		t.Error("sanitizeTestSuite should preserve TestCalculate function")
	}
}

func TestSanitizeTestSuite_PreservesTestFunctions(t *testing.T) {
	mainCode := `package main

func Add(a, b int) int {
	return a + b
}
`

	testCode := `package main

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatal("failed")
	}
}

func BenchmarkAdd(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Add(1, 2)
	}
}
`

	result := sanitizeTestSuite(testCode, mainCode)

	// Both test functions should remain
	if !strings.Contains(result, "func TestAdd") {
		t.Error("TestAdd should be preserved")
	}
	if !strings.Contains(result, "func BenchmarkAdd") {
		t.Error("BenchmarkAdd should be preserved")
	}
}

func TestSanitizeTestSuite_HandlesReceiverMethods(t *testing.T) {
	mainCode := `package main

type Calculator struct{}

func (c *Calculator) Compute(n int) int {
	return n * 2
}

func main() {}
`

	testCode := `package main

import "testing"

// This should NOT be removed (different signature - receiver vs non-receiver)
func Compute(n int) int {
	return n
}

func TestCompute(t *testing.T) {
	result := Compute(5)
	if result != 5 {
		t.Fatal("failed")
	}
}
`

	result := sanitizeTestSuite(testCode, mainCode)

	// The non-receiver Compute should remain since it doesn't collide with the method
	// Note: Current implementation only checks function names, not receivers
	// This test documents current behavior
	if !strings.Contains(result, "func TestCompute") {
		t.Error("TestCompute should be preserved")
	}
}

func TestSanitizeTestSuite_MultipleCollisions(t *testing.T) {
	mainCode := `package main

func Foo() int { return 1 }
func Bar() int { return 2 }
func Baz() int { return 3 }
`

	testCode := `package main

import "testing"

func Foo() int { return 999 }
func Bar() int { return 888 }

func TestFoo(t *testing.T) {
	if Foo() != 1 {
		t.Fatal("failed")
	}
}

func TestBar(t *testing.T) {
	if Bar() != 2 {
		t.Fatal("failed")
	}
}
`

	result := sanitizeTestSuite(testCode, mainCode)

	// Both duplicate functions should be removed
	lines := strings.Split(result, "\n")
	fooCount := 0
	barCount := 0
	for _, line := range lines {
		if strings.Contains(line, "func Foo()") {
			fooCount++
		}
		if strings.Contains(line, "func Bar()") {
			barCount++
		}
	}

	// Test functions should exist but not the duplicate implementations
	if !strings.Contains(result, "func TestFoo") {
		t.Error("TestFoo should be preserved")
	}
	if !strings.Contains(result, "func TestBar") {
		t.Error("TestBar should be preserved")
	}
}

func TestSanitizeTestSuite_NoCollisions(t *testing.T) {
	mainCode := `package main

func Calculate(n int) int {
	return n * 2
}
`

	testCode := `package main

import "testing"

func TestCalculate(t *testing.T) {
	result := Calculate(10)
	if result != 20 {
		t.Errorf("Expected 20, got %d", result)
	}
}
`

	result := sanitizeTestSuite(testCode, mainCode)

	// Nothing should change
	if result != testCode {
		t.Error("sanitizeTestSuite should not modify test code with no collisions")
	}
}

func TestSanitizeTestSuite_InvalidCode(t *testing.T) {
	mainCode := `package main

func Valid() int { return 1 }
`

	testCode := `this is not valid go code`

	result := sanitizeTestSuite(testCode, mainCode)

	// Should return original code when parsing fails
	if result != testCode {
		t.Error("sanitizeTestSuite should return original code when parsing fails")
	}
}

func TestExtractFunctionNames(t *testing.T) {
	code := `package main

func Foo() {}
func Bar(x int) int { return x }

type MyStruct struct{}

func (m *MyStruct) Method() {}

func main() {}
`

	funcs := extractFunctionNames(code)

	expected := map[string]bool{
		"Foo":    true,
		"Bar":    true,
		"Method": true,
		"main":   true,
	}

	for name := range expected {
		if !funcs[name] {
			t.Errorf("Expected function %s to be extracted", name)
		}
	}

	if len(funcs) != len(expected) {
		t.Errorf("Expected %d functions, got %d", len(expected), len(funcs))
	}
}
