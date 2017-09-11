package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/alecthomas/kingpin.v3-unstable"
)

func TestRelativePackagePath(t *testing.T) {
	var testcases = []struct {
		dir      string
		expected string
	}{
		{
			dir:      "/abs/path",
			expected: "/abs/path",
		},
		{
			dir:      ".",
			expected: ".",
		},
		{
			dir:      "./foo",
			expected: "./foo",
		},
		{
			dir:      "relative/path",
			expected: "./relative/path",
		},
	}

	for _, testcase := range testcases {
		assert.Equal(t, testcase.expected, relativePackagePath(testcase.dir))
	}
}

func TestResolvePathsNoPaths(t *testing.T) {
	paths := resolvePaths(nil, nil)
	assert.Equal(t, []string{"."}, paths)
}

func TestResolvePathsNoExpands(t *testing.T) {
	// Non-expanded paths should not be filtered by the skip path list
	paths := resolvePaths([]string{".", "foo", "foo/bar"}, []string{"foo/bar"})
	expected := []string{".", "./foo", "./foo/bar"}
	assert.Equal(t, expected, paths)
}

func TestResolvePathsWithExpands(t *testing.T) {
	tmpdir, cleanup := setupTempDir(t)
	defer cleanup()

	mkGoFile(t, tmpdir, "file.go")
	mkDir(t, tmpdir, "exclude")
	mkDir(t, tmpdir, "other", "exclude")
	mkDir(t, tmpdir, "include")
	mkDir(t, tmpdir, "include", "foo")
	mkDir(t, tmpdir, "duplicate")
	mkDir(t, tmpdir, ".exclude")
	mkDir(t, tmpdir, "include", ".exclude")
	mkDir(t, tmpdir, "_exclude")
	mkDir(t, tmpdir, "include", "_exclude")

	filterPaths := []string{"exclude", "other/exclude"}
	paths := resolvePaths([]string{"./...", "foo", "duplicate"}, filterPaths)

	expected := []string{
		".",
		"./duplicate",
		"./foo",
		"./include",
		"./include/foo",
	}
	assert.Equal(t, expected, paths)
}

func setupTempDir(t *testing.T) (string, func()) {
	tmpdir, err := ioutil.TempDir("", "test-expand-paths")
	require.NoError(t, err)

	oldwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpdir))

	return tmpdir, func() {
		os.RemoveAll(tmpdir)
		require.NoError(t, os.Chdir(oldwd))
	}
}

func mkDir(t *testing.T, paths ...string) {
	fullPath := filepath.Join(paths...)
	require.NoError(t, os.MkdirAll(fullPath, 0755))
	mkGoFile(t, fullPath, "file.go")
}

func mkGoFile(t *testing.T, path string, filename string) {
	content := []byte("package foo")
	err := ioutil.WriteFile(filepath.Join(path, filename), content, 0644)
	require.NoError(t, err)
}

func TestPathFilter(t *testing.T) {
	skip := []string{"exclude", "skip.go"}
	pathFilter := newPathFilter(skip)

	var testcases = []struct {
		path     string
		expected bool
	}{
		{path: "exclude", expected: true},
		{path: "something/skip.go", expected: true},
		{path: "skip.go", expected: true},
		{path: ".git", expected: true},
		{path: "_ignore", expected: true},
		{path: "include.go", expected: false},
		{path: ".", expected: false},
		{path: "..", expected: false},
	}

	for _, testcase := range testcases {
		assert.Equal(t, testcase.expected, pathFilter(testcase.path), testcase.path)
	}
}

func TestLoadConfigWithDeadline(t *testing.T) {
	originalConfig := *config
	defer func() { config = &originalConfig }()

	tmpfile, err := ioutil.TempFile("", "test-config")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.Write([]byte(`{"Deadline": "3m"}`))
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())

	filename := tmpfile.Name()
	err = loadConfig(nil, &kingpin.ParseElement{Value: &filename}, nil)
	require.NoError(t, err)

	require.Equal(t, 3*time.Minute, config.Deadline.Duration())
}

func TestDeadlineFlag(t *testing.T) {
	app := kingpin.New("test-app", "")
	setupFlags(app)
	_, err := app.Parse([]string{"--deadline", "2m"})
	require.NoError(t, err)
	require.Equal(t, 2*time.Minute, config.Deadline.Duration())
}

func TestAddPath(t *testing.T) {
	paths := []string{"existing"}
	assert.Equal(t, paths, addPath(paths, "existing"))
	expected := []string{"existing", "new"}
	assert.Equal(t, expected, addPath(paths, "new"))
}

func TestSetupFlagsLinterFlag(t *testing.T) {
	originalConfig := *config
	defer func() { config = &originalConfig }()

	app := kingpin.New("test-app", "")
	setupFlags(app)
	_, err := app.Parse([]string{"--linter", "a:b:c"})
	require.NoError(t, err)
	linter, ok := config.Linters["a"]
	assert.True(t, ok)
	assert.Equal(t, "b", linter.Command)
	assert.Equal(t, "c", linter.Pattern)
}

func TestSetupFlagsConfigWithLinterString(t *testing.T) {
	originalConfig := *config
	defer func() { config = &originalConfig }()

	tmpfile, err := ioutil.TempFile("", "test-config")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.Write([]byte(`{"Linters": {"linter": "command:path"} }`))
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())

	app := kingpin.New("test-app", "")
	setupFlags(app)

	_, err = app.Parse([]string{"--config", tmpfile.Name()})
	require.NoError(t, err)
	linter, ok := config.Linters["linter"]
	assert.True(t, ok)
	assert.Equal(t, "command", linter.Command)
	assert.Equal(t, "path", linter.Pattern)
}

func TestSetupFlagsConfigWithLinterMap(t *testing.T) {
	originalConfig := *config
	defer func() { config = &originalConfig }()

	tmpfile, err := ioutil.TempFile("", "test-config")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.Write([]byte(`{"Linters":
		{"linter":
			{ "Command": "command" }}}`))
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())

	app := kingpin.New("test-app", "")
	setupFlags(app)

	_, err = app.Parse([]string{"--config", tmpfile.Name()})
	require.NoError(t, err)
	linter, ok := config.Linters["linter"]
	assert.True(t, ok)
	assert.Equal(t, "command", linter.Command)
	assert.Equal(t, "", linter.Pattern)
}

func TestSetupFlagsConfigAndLinterFlag(t *testing.T) {
	originalConfig := *config
	defer func() { config = &originalConfig }()

	tmpfile, err := ioutil.TempFile("", "test-config")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.Write([]byte(`{"Linters":
		{"linter": { "Command": "some-command" }}}`))
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())

	app := kingpin.New("test-app", "")
	setupFlags(app)

	_, err = app.Parse([]string{
		"--config", tmpfile.Name(),
		"--linter", "linter:command:pattern"})
	require.NoError(t, err)
	linter, ok := config.Linters["linter"]
	assert.True(t, ok)
	assert.Equal(t, "command", linter.Command)
	assert.Equal(t, "pattern", linter.Pattern)
}
