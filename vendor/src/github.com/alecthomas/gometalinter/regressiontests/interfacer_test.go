package regressiontests

import "testing"

func TestInterfacer(t *testing.T) {
	t.Parallel()
	expected := Issues{
		{Linter: "interfacer", Severity: "warning", Path: "test.go", Line: 5, Col: 8, Message: "r can be io.Closer"},
	}
	ExpectIssues(t, "interfacer", `package main

import "os"

func f(r *os.File) {
	r.Close()
}

func main() {
}
`, expected)
}
