package config

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHttpAddress_ParseGood(t *testing.T) {
	address, err := HttpAddress("http://localhost:123")
	assert.NoError(t, err)
	assert.Equal(t, "localhost:123", address.Address)
	assert.Equal(t, "tcp", address.Network())
}

func TestHttpAddress_ParseBad(t *testing.T) {
	_, err := HttpAddress(":")
	assert.Error(t, err)
}

func TestUnixSocketAddress_Network(t *testing.T) {
	address := UnixSocketAddress("/tmp", fs.FileMode(0755))
	assert.Equal(t, "unix", address.Network())
}
