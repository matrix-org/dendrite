package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLinterWithCustomLinter(t *testing.T) {
	config := LinterConfig{
		Command: "/usr/bin/custom",
		Pattern: "path",
	}
	linter, err := NewLinter(config)
	require.NoError(t, err)
	assert.NotNil(t, linter.LinterConfig.PartitionStrategy)
}

func TestGetLinterByName(t *testing.T) {
	config := LinterConfig{
		Command: "aligncheck",
		Pattern: "path",
	}
	overrideConfig := getLinterByName(config.Command, config)
	require.Equal(t, config.Command, overrideConfig.Command)
	require.Equal(t, config.Pattern, overrideConfig.Pattern)
}
