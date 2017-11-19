package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinterConfigUnmarshalJSON(t *testing.T) {
	source := `{
		"Command": "/bin/custom",
		"PartitionStrategy": "directories"
	}`
	var config StringOrLinterConfig
	err := json.Unmarshal([]byte(source), &config)
	require.NoError(t, err)
	assert.Equal(t, "/bin/custom", config.Command)
	assert.Equal(t, functionName(partitionPathsAsDirectories), functionName(config.PartitionStrategy))
}
