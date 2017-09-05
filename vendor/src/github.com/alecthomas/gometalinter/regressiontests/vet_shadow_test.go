package regressiontests

import "testing"

func TestVetShadow(t *testing.T) {
	t.Parallel()
	source := `package test

func test(mystructs []*MyStruct) *MyStruct {
	var foo *MyStruct
	for _, mystruct := range mystructs {
		foo := mystruct
	}
	return foo
}
`
	expected := Issues{
		{Linter: "vetshadow", Severity: "warning", Path: "test.go", Line: 6, Col: 0, Message: "declaration of \"foo\" shadows declaration at test.go:4"},
	}
	ExpectIssues(t, "vetshadow", source, expected)
}
