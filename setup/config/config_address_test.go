package config

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHttpAddress_ParseGood(t *testing.T) {
	address, err := HTTPAddress("http://localhost:123")
	assert.NoError(t, err)
	assert.Equal(t, "localhost:123", address.Address)
	assert.Equal(t, "tcp", address.Network())
}

func TestHttpAddress_ParseBad(t *testing.T) {
	_, err := HTTPAddress(":")
	assert.Error(t, err)
}

func TestUnixSocketAddress_Network(t *testing.T) {
	address, err := UnixSocketAddress("/tmp", "0755")
	assert.NoError(t, err)
	assert.Equal(t, "unix", address.Network())
}

func TestUnixSocketAddress_Permission_LeadingZero_Ok(t *testing.T) {
	address, err := UnixSocketAddress("/tmp", "0755")
	assert.NoError(t, err)
	assert.Equal(t, fs.FileMode(0755), address.UnixSocketPermission)
}

func TestUnixSocketAddress_Permission_NoLeadingZero_Ok(t *testing.T) {
	address, err := UnixSocketAddress("/tmp", "755")
	assert.NoError(t, err)
	assert.Equal(t, fs.FileMode(0755), address.UnixSocketPermission)
}

func TestUnixSocketAddress_Permission_NonOctal_Bad(t *testing.T) {
	_, err := UnixSocketAddress("/tmp", "855")
	assert.Error(t, err)
}
