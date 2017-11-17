package regressiontests

import "testing"

func TestMaligned(t *testing.T) {
	t.Parallel()
	source := `package test

type unaligned struct {
	a uint16
	b uint64
	c uint16

}
`
	expected := Issues{
		{Linter: "maligned", Severity: "warning", Path: "test.go", Line: 3, Col: 16, Message: "struct of size 24 could be 16"},
	}
	ExpectIssues(t, "maligned", source, expected)
}
