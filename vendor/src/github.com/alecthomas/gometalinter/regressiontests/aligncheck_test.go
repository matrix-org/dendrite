package regressiontests

import "testing"

func TestAlignCheck(t *testing.T) {
	t.Parallel()
	source := `package test

type unaligned struct {
	a uint16
	b uint64
	c uint16

}
`
	expected := Issues{
		{Linter: "aligncheck", Severity: "warning", Path: "test.go", Line: 3, Col: 6, Message: "struct unaligned could have size 16 (currently 24)"},
	}
	ExpectIssues(t, "aligncheck", source, expected)
}
