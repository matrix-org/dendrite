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

package thrift

import (
	"bytes"
	"io"
	"testing"

	"github.com/uber/tchannel-go/testutils/testreader"
	"github.com/uber/tchannel-go/testutils/testwriter"

	"github.com/stretchr/testify/assert"
)

func writeByte(writer io.Writer, b byte) error {
	protocol := getProtocolWriter(writer)
	return protocol.transport.WriteByte(b)
}

func TestWriteByteSuccess(t *testing.T) {
	writer := &bytes.Buffer{}
	assert.NoError(t, writeByte(writer, 'a'), "WriteByte failed")
	assert.NoError(t, writeByte(writer, 'b'), "WriteByte failed")
	assert.NoError(t, writeByte(writer, 'c'), "WriteByte failed")
	assert.Equal(t, []byte("abc"), writer.Bytes(), "Written bytes mismatch")
}

func TestWriteByteFailed(t *testing.T) {
	buf := &bytes.Buffer{}
	writer := io.MultiWriter(testwriter.Limited(2), buf)
	assert.NoError(t, writeByte(writer, 'a'), "WriteByte failed")
	assert.NoError(t, writeByte(writer, 'b'), "WriteByte failed")
	assert.Error(t, writeByte(writer, 'c'), "WriteByte should fail due to lack of space")
	assert.Equal(t, []byte("ab"), buf.Bytes(), "Written bytes mismatch")
}

func TestReadByte0Byte(t *testing.T) {
	chunkWriter, chunkReader := testreader.ChunkReader()
	reader := getProtocolReader(chunkReader)

	chunkWriter <- []byte{}
	chunkWriter <- []byte{}
	chunkWriter <- []byte{}
	chunkWriter <- []byte("abc")
	close(chunkWriter)

	b, err := reader.transport.ReadByte()
	assert.NoError(t, err, "ReadByte should ignore 0 byte reads")
	assert.EqualValues(t, 'a', b)

	b, err = reader.transport.ReadByte()
	assert.NoError(t, err, "ReadByte failed")
	assert.EqualValues(t, 'b', b)

	b, err = reader.transport.ReadByte()
	assert.NoError(t, err, "ReadByte failed")
	assert.EqualValues(t, 'c', b)

	b, err = reader.transport.ReadByte()
	assert.Equal(t, io.EOF, err, "ReadByte should EOF")
}
