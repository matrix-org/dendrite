package regressiontests

import "testing"

func TestLLL(t *testing.T) {
	t.Parallel()
	source := `package test
// This is a really long line full of text that is uninteresting in the extreme. Also we're just trying to make it here.
`
	expected := Issues{
		{Linter: "lll", Severity: "warning", Path: "test.go", Line: 2, Col: 0, Message: "line is 120 characters"},
	}
	ExpectIssues(t, "lll", source, expected)
}
