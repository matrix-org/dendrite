package regressiontests

import (
	"fmt"
	"testing"

	"github.com/gotestyourself/gotestyourself/fs"
	"github.com/stretchr/testify/assert"
)

func TestGas(t *testing.T) {
	t.Parallel()
	dir := fs.NewDir(t, "test-gas",
		fs.WithFile("file.go", gasFileErrorUnhandled("root")),
		fs.WithDir("sub",
			fs.WithFile("file.go", gasFileErrorUnhandled("sub"))))
	defer dir.Remove()
	expected := Issues{
		{Linter: "gas", Severity: "warning", Path: "file.go", Line: 3, Col: 0, Message: "Errors unhandled.,LOW,HIGH"},
		{Linter: "gas", Severity: "warning", Path: "sub/file.go", Line: 3, Col: 0, Message: "Errors unhandled.,LOW,HIGH"},
	}
	actual := RunLinter(t, "gas", dir.Path())
	assert.Equal(t, expected, actual)
}

func gasFileErrorUnhandled(pkg string) string {
	return fmt.Sprintf(`package %s
	func badFunction() string {
		u, _ := ErrorHandle()
		return u
	}
	
	func ErrorHandle() (u string, err error) {
		return u
	}
	`, pkg)
}
