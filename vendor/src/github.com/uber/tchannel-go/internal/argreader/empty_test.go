// Copyright (c) 2015 Uber Technologies, Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package argreader

import (
	"bytes"
	"testing"

	"github.com/uber/tchannel-go/testutils/testreader"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureEmptySuccess(t *testing.T) {
	reader := bytes.NewReader(nil)
	err := EnsureEmpty(reader, "success")
	require.NoError(t, err, "ensureEmpty should succeed with empty reader")
}

func TestEnsureEmptyHasBytes(t *testing.T) {
	reader := bytes.NewReader([]byte{1, 2, 3})
	err := EnsureEmpty(reader, "T")
	require.Error(t, err, "ensureEmpty should fail when there's bytes")
	assert.Equal(t, err.Error(), "found unexpected bytes after T, found (upto 128 bytes): 010203")
}

func TestEnsureEmptyError(t *testing.T) {
	control, reader := testreader.ChunkReader()
	control <- nil
	close(control)

	err := EnsureEmpty(reader, "has bytes")
	require.Error(t, err, "ensureEmpty should fail when there's an error")
	assert.Equal(t, testreader.ErrUser, err, "Unexpected error")
}
