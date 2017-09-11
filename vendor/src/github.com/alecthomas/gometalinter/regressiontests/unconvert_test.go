package regressiontests

import "testing"

func TestUnconvert(t *testing.T) {
	t.Parallel()
	source := `package test

func test() {
	var a int64
	b := int64(a)
	println(b)
}`
	expected := Issues{
		{Linter: "unconvert", Severity: "warning", Path: "test.go", Line: 5, Col: 12, Message: "unnecessary conversion"},
	}
	ExpectIssues(t, "unconvert", source, expected)
}
