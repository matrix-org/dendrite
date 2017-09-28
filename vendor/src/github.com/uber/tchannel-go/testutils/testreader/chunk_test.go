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

package testreader

import (
	"io"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChunkReader0ByteRead(t *testing.T) {
	writer, reader := ChunkReader()

	writer <- []byte{}
	writer <- []byte{'a'}
	close(writer)

	buf := make([]byte, 1)
	n, err := reader.Read(buf)
	assert.NoError(t, err, "Read should not fail")
	assert.Equal(t, 0, n, "Read should not read any bytes")

	n, err = reader.Read(buf)
	assert.NoError(t, err, "Read should not fail")
	assert.Equal(t, 1, n, "Read should read one byte")
	assert.EqualValues(t, 'a', buf[0], "Read did not read correct byte")

	n, err = reader.Read(buf)
	assert.Equal(t, io.EOF, err, "Read should EOF")
	assert.Equal(t, 0, n, "Read should not read any bytes")
}

func TestChunkReader(t *testing.T) {
	writer, reader := ChunkReader()

	writer <- []byte{1, 2}
	writer <- []byte{3}
	writer <- nil
	writer <- []byte{4}
	writer <- []byte{}
	writer <- []byte{5}
	writer <- []byte{}
	writer <- []byte{6}
	writer <- []byte{}
	close(writer)

	buf, err := ioutil.ReadAll(reader)
	assert.Equal(t, ErrUser, err, "Expected error after initial bytes")
	assert.Equal(t, []byte{1, 2, 3}, buf, "Unexpected bytes")

	buf, err = ioutil.ReadAll(reader)
	assert.NoError(t, err, "Reader shouldn't fail on second set of bytes")
	assert.Equal(t, []byte{4, 5, 6}, buf, "Unexpected bytes")
}
