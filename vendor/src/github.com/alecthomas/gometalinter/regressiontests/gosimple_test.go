package regressiontests

import "testing"

func TestGoSimple(t *testing.T) {
	t.Parallel()
	source := `package test

func a(ok bool, ch chan bool) {
	select {
	case <- ch:
	}

	for {
		select {
		case <- ch:
		}
	}

	if ok == true {
	}
}
`
	expected := Issues{
		{Linter: "gosimple", Severity: "warning", Path: "test.go", Line: 8, Col: 2, Message: "should use for range instead of for { select {} } (S1000)"},
		{Linter: "gosimple", Severity: "warning", Path: "test.go", Line: 14, Col: 5, Message: "should omit comparison to bool constant, can be simplified to ok (S1002)"},
		{Linter: "gosimple", Severity: "warning", Path: "test.go", Line: 4, Col: 2, Message: "should use a simple channel send/receive instead of select with a single case (S1000)"},
	}
	ExpectIssues(t, "gosimple", source, expected)
}
