package regressiontests

import "testing"

func TestErrcheck(t *testing.T) {
	t.Parallel()
	source := `package moo

func f() error { return nil}
func test() { f() }
`
	expected := Issues{
		{Linter: "errcheck", Severity: "warning", Path: "test.go", Line: 4, Col: 16, Message: "error return value not checked (func test() { f() })"},
	}
	ExpectIssues(t, "errcheck", source, expected)
}
