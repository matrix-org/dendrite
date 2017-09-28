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

package typed

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/tchannel-go/testutils/testreader"
)

func nString(n int) []byte {
	buf := make([]byte, n)
	reader := testreader.Looper([]byte{'a', 'b', 'c', 'd', 'e'})
	io.ReadFull(reader, buf)
	return buf
}

func TestReader(t *testing.T) {
	s1 := nString(10)
	s2 := nString(800)

	var buf []byte
	buf = append(buf, 0, 1)       // uint16, 1
	buf = append(buf, 0xff, 0xff) // uint16, 65535
	buf = append(buf, 0, 10)      // uint16, 10
	buf = append(buf, s1...)      // string, 10 bytes
	buf = append(buf, 3, 32)      // uint16, 800
	buf = append(buf, s2...)      // string, 800 bytes
	buf = append(buf, 0, 10)      // uint16, 10

	reader := NewReader(bytes.NewReader(buf))

	assert.Equal(t, uint16(1), reader.ReadUint16())
	assert.Equal(t, uint16(65535), reader.ReadUint16())
	assert.Equal(t, string(s1), reader.ReadLen16String())
	assert.Equal(t, string(s2), reader.ReadLen16String())
	assert.Equal(t, uint16(10), reader.ReadUint16())
}

func TestReaderErr(t *testing.T) {
	tests := []struct {
		chunks     [][]byte
		validation func(reader *Reader)
	}{
		{
			chunks: [][]byte{
				{0, 1},
				nil,
				{2, 3},
			},
			validation: func(reader *Reader) {
				assert.Equal(t, uint16(1), reader.ReadUint16(), "Read unexpected value")
				assert.Equal(t, uint16(0), reader.ReadUint16(), "Expected default value")
			},
		},
		{
			chunks: [][]byte{
				{0, 4},
				[]byte("test"),
				nil,
				{'A', 'b'},
			},
			validation: func(reader *Reader) {
				assert.Equal(t, "test", reader.ReadLen16String(), "Read unexpected value")
				assert.Equal(t, "", reader.ReadString(2), "Expected default value")
			},
		},
	}

	for _, tt := range tests {
		writer, chunkReader := testreader.ChunkReader()
		reader := NewReader(chunkReader)
		defer reader.Release()

		for _, chunk := range tt.chunks {
			writer <- chunk
		}
		close(writer)

		tt.validation(reader)
		// Once there's an error, all further calls should fail.
		assert.Equal(t, testreader.ErrUser, reader.Err(), "Unexpected error")
		assert.Equal(t, uint16(0), reader.ReadUint16(), "Expected default value")
		assert.Equal(t, "", reader.ReadString(1), "Expected default value")
		assert.Equal(t, "", reader.ReadLen16String(), "Expected default value")
		assert.Equal(t, testreader.ErrUser, reader.Err(), "Unexpected error")
	}
}
