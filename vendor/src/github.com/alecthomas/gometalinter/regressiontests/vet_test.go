package regressiontests

import "testing"

func TestVet(t *testing.T) {
	t.Parallel()
	expected := Issues{
		{Linter: "vet", Severity: "error", Path: "test.go", Line: 7, Col: 0, Message: "missing argument for Printf(\"%d\"): format reads arg 1, have only 0 args"},
		{Linter: "vet", Severity: "error", Path: "test.go", Line: 7, Col: 0, Message: "unreachable code"},
	}
	ExpectIssues(t, "vet", `package main

import "fmt"

func main() {
	return
	fmt.Printf("%d")
}
`, expected)
}
