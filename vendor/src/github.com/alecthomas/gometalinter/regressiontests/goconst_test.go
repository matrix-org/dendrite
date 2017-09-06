package regressiontests

import "testing"

func TestGoconst(t *testing.T) {
	t.Parallel()
	source := `package test
func a() {
	foo := "bar"
}
func b() {
	bar := "bar"
}
`
	expected := Issues{
		{Linter: "goconst", Severity: "warning", Path: "test.go", Line: 3, Col: 9, Message: `1 other occurrence(s) of "bar" found in: test.go:6:9`},
		{Linter: "goconst", Severity: "warning", Path: "test.go", Line: 6, Col: 9, Message: `1 other occurrence(s) of "bar" found in: test.go:3:9`},
	}
	ExpectIssues(t, "goconst", source, expected, "--min-occurrences", "2")
}
