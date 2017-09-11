package regressiontests

import "testing"

func TestMegaCheck(t *testing.T) {
	t.Parallel()
	source := `package test

func f() {
	var ok bool
	if ok == true {
	}
}
`
	expected := Issues{
		{Linter: "megacheck", Severity: "warning", Path: "test.go", Line: 3, Col: 6, Message: "func f is unused (U1000)"},
		{Linter: "megacheck", Severity: "warning", Path: "test.go", Line: 5, Col: 2, Message: "empty branch (SA9003)"},
		{Linter: "megacheck", Severity: "warning", Path: "test.go", Line: 5, Col: 5, Message: "should omit comparison to bool constant, can be simplified to ok (S1002)"},
	}
	ExpectIssues(t, "megacheck", source, expected)
}
