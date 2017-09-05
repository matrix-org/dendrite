package regressiontests

import "testing"

func TestGolint(t *testing.T) {
	t.Parallel()
	source := `
package test

type Foo int
`
	expected := Issues{
		{Linter: "golint", Severity: "warning", Path: "test.go", Line: 4, Col: 6, Message: "exported type Foo should have comment or be unexported"},
	}
	ExpectIssues(t, "golint", source, expected)
}
