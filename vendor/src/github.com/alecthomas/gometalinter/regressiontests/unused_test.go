package regressiontests

import "testing"

func TestUnused(t *testing.T) {
	t.Parallel()
	source := `package test

var v int = 10

func f() {
}
`
	expected := Issues{
		{Linter: "unused", Severity: "warning", Path: "test.go", Line: 3, Col: 5, Message: "var v is unused (U1000)"},
		{Linter: "unused", Severity: "warning", Path: "test.go", Line: 5, Col: 6, Message: "func f is unused (U1000)"},
	}
	ExpectIssues(t, "unused", source, expected)
}
