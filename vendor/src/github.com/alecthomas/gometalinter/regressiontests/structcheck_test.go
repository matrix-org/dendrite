package regressiontests

import "testing"

func TestStructcheck(t *testing.T) {
	t.Parallel()
	source := `package test

type test struct {
	unused int
}
`
	expected := Issues{
		{Linter: "structcheck", Severity: "warning", Path: "test.go", Line: 4, Col: 2, Message: "unused struct field github.com/alecthomas/gometalinter/regressiontests/.test.unused"},
	}
	ExpectIssues(t, "structcheck", source, expected)
}
