package regressiontests

import "testing"

func TestIneffassign(t *testing.T) {
	t.Parallel()
	source := `package test

func test() {
	a := 1
}`
	expected := Issues{
		{Linter: "ineffassign", Severity: "warning", Path: "test.go", Line: 4, Col: 2, Message: "ineffectual assignment to a"},
	}
	ExpectIssues(t, "ineffassign", source, expected)
}
