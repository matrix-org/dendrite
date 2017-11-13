package main

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLinterWithCustomLinter(t *testing.T) {
	config := LinterConfig{
		Command: "/usr/bin/custom",
		Pattern: "path",
	}
	linter, err := NewLinter("thename", config)
	require.NoError(t, err)
	assert.Equal(t, functionName(partitionPathsAsDirectories), functionName(linter.LinterConfig.PartitionStrategy))
	assert.Equal(t, "(?m:path)", linter.regex.String())
	assert.Equal(t, "thename", linter.Name)
	assert.Equal(t, config.Command, linter.Command)
}

func TestGetLinterByName(t *testing.T) {
	config := LinterConfig{
		Command:           "maligned",
		Pattern:           "path",
		InstallFrom:       "./install/path",
		PartitionStrategy: partitionPathsAsDirectories,
		IsFast:            true,
	}
	overrideConfig := getLinterByName(config.Command, config)
	assert.Equal(t, config.Command, overrideConfig.Command)
	assert.Equal(t, config.Pattern, overrideConfig.Pattern)
	assert.Equal(t, config.InstallFrom, overrideConfig.InstallFrom)
	assert.Equal(t, functionName(config.PartitionStrategy), functionName(overrideConfig.PartitionStrategy))
	assert.Equal(t, config.IsFast, overrideConfig.IsFast)
}

func TestValidateLinters(t *testing.T) {
	originalConfig := *config
	defer func() { config = &originalConfig }()

	config = &Config{
		Enable: []string{"_dummylinter_"},
	}

	err := validateLinters(lintersFromConfig(config), config)
	require.Error(t, err, "expected unknown linter error for _dummylinter_")

	config = &Config{
		Enable: defaultEnabled(),
	}
	err = validateLinters(lintersFromConfig(config), config)
	require.NoError(t, err)
}

func functionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}
