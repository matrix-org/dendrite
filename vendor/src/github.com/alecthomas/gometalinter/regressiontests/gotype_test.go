package regressiontests

import "testing"

func TestGoType(t *testing.T) {
	t.Parallel()
	source := `package test

func test() {
	var foo string
}
`
	expected := Issues{
		{Linter: "gotype", Severity: "error", Path: "test.go", Line: 4, Col: 6, Message: "foo declared but not used"},
	}
	ExpectIssues(t, "gotype", source, expected)
}
