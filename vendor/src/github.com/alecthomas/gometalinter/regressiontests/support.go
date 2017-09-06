package regressiontests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Issue struct {
	Linter   string `json:"linter"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	Message  string `json:"message"`
}

func (i *Issue) String() string {
	col := ""
	if i.Col != 0 {
		col = fmt.Sprintf("%d", i.Col)
	}
	return fmt.Sprintf("%s:%d:%s:%s: %s (%s)", strings.TrimSpace(i.Path), i.Line, col, i.Severity, strings.TrimSpace(i.Message), i.Linter)
}

type Issues []Issue

func (e Issues) Len() int           { return len(e) }
func (e Issues) Swap(i, j int)      { e[i], e[j] = e[j], e[i] }
func (e Issues) Less(i, j int) bool { return e[i].String() < e[j].String() }

// ExpectIssues runs gometalinter and expects it to generate exactly the
// issues provided.
func ExpectIssues(t *testing.T, linter string, source string, expected Issues, extraFlags ...string) {
	// Write source to temporary directory.
	dir, err := ioutil.TempDir(".", "gometalinter-")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	testFile := filepath.Join(dir, "test.go")
	err = ioutil.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	// Run gometalinter.
	binary, cleanup := buildBinary(t)
	defer cleanup()
	args := []string{"-d", "--disable-all", "--enable", linter, "--json", dir}
	args = append(args, extraFlags...)
	cmd := exec.Command(binary, args...)
	errBuffer := new(bytes.Buffer)
	cmd.Stderr = errBuffer
	require.NoError(t, err)

	output, _ := cmd.Output()
	var actual Issues
	err = json.Unmarshal(output, &actual)
	if !assert.NoError(t, err) {
		fmt.Printf("Stderr: %s\n", errBuffer)
		fmt.Printf("Output: %s\n", output)
		return
	}

	// Remove output from other linters.
	actualForLinter := Issues{}
	for _, issue := range actual {
		if issue.Linter == linter || linter == "" {
			// Normalise path.
			issue.Path = "test.go"
			issue.Message = strings.Replace(issue.Message, testFile, "test.go", -1)
			issue.Message = strings.Replace(issue.Message, dir, "", -1)
			actualForLinter = append(actualForLinter, issue)
		}
	}
	sort.Sort(expected)
	sort.Sort(actualForLinter)

	if !assert.Equal(t, expected, actualForLinter) {
		fmt.Printf("Stderr: %s\n", errBuffer)
		fmt.Printf("Output: %s\n", output)
	}
}

func buildBinary(t *testing.T) (string, func()) {
	tmpdir, err := ioutil.TempDir("", "regression-test")
	require.NoError(t, err)
	path := filepath.Join(tmpdir, "binary")
	cmd := exec.Command("go", "build", "-o", path, "..")
	require.NoError(t, cmd.Run())
	return path, func() { os.RemoveAll(tmpdir) }
}
