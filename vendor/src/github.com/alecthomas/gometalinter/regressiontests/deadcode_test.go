package regressiontests

import "testing"

func TestDeadcode(t *testing.T) {
	t.Parallel()
	source := `package test

func test() {
	return
	println("hello")
}
`
	expected := Issues{
		{Linter: "deadcode", Severity: "warning", Path: "test.go", Line: 3, Col: 1, Message: "test is unused"},
	}
	ExpectIssues(t, "deadcode", source, expected)
}
