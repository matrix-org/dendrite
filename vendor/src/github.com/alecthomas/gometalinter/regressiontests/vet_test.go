package regressiontests

import (
	"testing"

	"github.com/gotestyourself/gotestyourself/fs"
	"github.com/stretchr/testify/assert"
)

func TestVet(t *testing.T) {
	t.Parallel()

	dir := fs.NewDir(t, "test-vet",
		fs.WithFile("file.go", vetFile("root")),
		fs.WithFile("file_test.go", vetExternalPackageFile("root_test")),
		fs.WithDir("sub",
			fs.WithFile("file.go", vetFile("sub"))),
		fs.WithDir("excluded",
			fs.WithFile("file.go", vetFile("excluded"))))
	defer dir.Remove()

	expected := Issues{
		{Linter: "vet", Severity: "error", Path: "file.go", Line: 7, Col: 0, Message: "missing argument for Printf(\"%d\"): format reads arg 1, have only 0 args"},
		{Linter: "vet", Severity: "error", Path: "file.go", Line: 7, Col: 0, Message: "unreachable code"},
		{Linter: "vet", Severity: "error", Path: "file_test.go", Line: 7, Col: 0, Message: "unreachable code"},
		{Linter: "vet", Severity: "error", Path: "sub/file.go", Line: 7, Col: 0, Message: "missing argument for Printf(\"%d\"): format reads arg 1, have only 0 args"},
		{Linter: "vet", Severity: "error", Path: "sub/file.go", Line: 7, Col: 0, Message: "unreachable code"},
	}
	actual := RunLinter(t, "vet", dir.Path(), "--skip=excluded")
	assert.Equal(t, expected, actual)
}

func vetFile(pkg string) string {
	return `package ` + pkg + `

import "fmt"

func Something() {
	return
	fmt.Printf("%d")
}
`
}

func vetExternalPackageFile(pkg string) string {
	return `package ` + pkg + `

import "fmt"

func ExampleSomething() {
	return
	root.Something()
}
`
}
