// Copyright 2024 New Vector Ltd.
// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package conduit

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

var TestBuf = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

type TestNetConn struct {
	net.Conn
	shouldFail bool
}

func (t *TestNetConn) Read(b []byte) (int, error) {
	if t.shouldFail {
		return 0, fmt.Errorf("Failed")
	} else {
		n := copy(b, TestBuf)
		return n, nil
	}
}

func (t *TestNetConn) Write(b []byte) (int, error) {
	if t.shouldFail {
		return 0, fmt.Errorf("Failed")
	} else {
		return len(b), nil
	}
}

func (t *TestNetConn) Close() error {
	if t.shouldFail {
		return fmt.Errorf("Failed")
	} else {
		return nil
	}
}

func TestConduitStoresPort(t *testing.T) {
	conduit := Conduit{port: 7}
	assert.Equal(t, 7, conduit.Port())
}

func TestConduitRead(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{}}
	b := make([]byte, len(TestBuf))
	bytes, err := conduit.Read(b)
	assert.NoError(t, err)
	assert.Equal(t, len(TestBuf), bytes)
	assert.Equal(t, TestBuf, b)
}

func TestConduitReadCopy(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{}}
	result, err := conduit.ReadCopy()
	assert.NoError(t, err)
	assert.Equal(t, TestBuf, result)
}

func TestConduitWrite(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{}}
	bytes, err := conduit.Write(TestBuf)
	assert.NoError(t, err)
	assert.Equal(t, len(TestBuf), bytes)
}

func TestConduitClose(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{}}
	err := conduit.Close()
	assert.NoError(t, err)
	assert.True(t, conduit.closed.Load())
}

func TestConduitReadClosed(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{}}
	err := conduit.Close()
	assert.NoError(t, err)
	b := make([]byte, len(TestBuf))
	_, err = conduit.Read(b)
	assert.Error(t, err)
}

func TestConduitReadCopyClosed(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{}}
	err := conduit.Close()
	assert.NoError(t, err)
	_, err = conduit.ReadCopy()
	assert.Error(t, err)
}

func TestConduitWriteClosed(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{}}
	err := conduit.Close()
	assert.NoError(t, err)
	_, err = conduit.Write(TestBuf)
	assert.Error(t, err)
}

func TestConduitReadCopyFails(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{shouldFail: true}}
	_, err := conduit.ReadCopy()
	assert.Error(t, err)
}
