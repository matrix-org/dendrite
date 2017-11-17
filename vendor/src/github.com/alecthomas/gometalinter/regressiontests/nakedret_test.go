package regressiontests

import "testing"

func TestNakedret(t *testing.T) {
	t.Parallel()
	source := `package test

func shortFunc() (r uint32) {
	r = r + r
	return
}
	
func longFunc() (r uint32) {
	r = r + r
	r = r - r
	r = r * r
	r = r / r
	r = r % r
	r = r^r
	r = r&r
	return
}	
`
	expected := Issues{
		{Linter: "nakedret", Severity: "warning", Path: "test.go", Line: 16, Message: "longFunc naked returns on 9 line function "},
	}
	ExpectIssues(t, "nakedret", source, expected)
}
