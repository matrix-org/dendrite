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
	"io/ioutil"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
)

var headers = map[string]string{
	"header1": "value1",
	"header2": "value2",
	"header3": "value1",
	"header4": "value2",
	"header5": "value1",
	"header6": "value2",
	"header7": "value1",
	"header8": "value2",
	"header9": "value1",
	"header0": "value2",
}

var headerTests = []struct {
	m         map[string]string
	encoding  []byte
	encoding2 []byte
}{
	{
		m:        nil,
		encoding: []byte{0, 0},
	},
	{
		m:        make(map[string]string),
		encoding: []byte{0, 0},
	},
	{
		m: map[string]string{
			"k": "v",
		},
		encoding: []byte{
			0, 1, /* number of headers */
			0, 1, /* length of key */
			'k',
			0, 1, /* length of value */
			'v',
		},
	},
	{
		m: map[string]string{
			"": "",
		},
		encoding: []byte{
			0, 1, /* number of headers */
			0, 0,
			0, 0,
		},
	},
	{
		m: map[string]string{
			"k1": "v12",
			"k2": "v34",
		},
		encoding: []byte{
			0, 2, /* number of headers */
			0, 2, /* length of key */
			'k', '2',
			0, 3, /* length of value */
			'v', '3', '4',
			0, 2, /* length of key */
			'k', '1',
			0, 3, /* length of value */
			'v', '1', '2',
		},
		encoding2: []byte{
			0, 2, /* number of headers */
			0, 2, /* length of key */
			'k', '1',
			0, 3, /* length of value */
			'v', '1', '2',
			0, 2, /* length of key */
			'k', '2',
			0, 3, /* length of value */
			'v', '3', '4',
		},
	},
}

func TestWriteHeadersSuccessful(t *testing.T) {
	for _, tt := range headerTests {
		buf := &bytes.Buffer{}
		err := WriteHeaders(buf, tt.m)
		assert.NoError(t, err, "WriteHeaders failed")

		// Writes iterate over the map in an undefined order, so we might get
		// encoding or encoding2. If it's not encoding, assert that it's encoding2.
		if !bytes.Equal(tt.encoding, buf.Bytes()) {
			assert.Equal(t, tt.encoding2, buf.Bytes(), "Unexpected bytes")
		}
	}
}

func TestReadHeadersSuccessful(t *testing.T) {
	for _, tt := range headerTests {
		// when the bytes are {0, 0}, we always return nil.
		if tt.m != nil && len(tt.m) == 0 {
			continue
		}

		reader := iotest.OneByteReader(bytes.NewReader(tt.encoding))
		got, err := ReadHeaders(reader)
		assert.NoError(t, err, "ReadHeaders failed")
		assert.Equal(t, tt.m, got, "Map mismatch")

		if tt.encoding2 != nil {
			reader := iotest.OneByteReader(bytes.NewReader(tt.encoding2))
			got, err := ReadHeaders(reader)
			assert.NoError(t, err, "ReadHeaders failed")
			assert.Equal(t, tt.m, got, "Map mismatch")
		}
	}
}

func TestReadHeadersLeftoverBytes(t *testing.T) {
	buf := []byte{0, 0, 1, 2, 3}
	r := bytes.NewReader(buf)
	headers, err := ReadHeaders(r)
	assert.NoError(t, err, "ReadHeaders failed")
	assert.Equal(t, map[string]string(nil), headers, "Headers mismatch")

	leftover, err := ioutil.ReadAll(r)
	assert.NoError(t, err, "ReadAll failed")
	assert.Equal(t, []byte{1, 2, 3}, leftover, "Reader consumed leftover bytes")
}

func BenchmarkWriteHeaders(b *testing.B) {
	for i := 0; i < b.N; i++ {
		WriteHeaders(ioutil.Discard, headers)
	}
}

func BenchmarkReadHeaders(b *testing.B) {
	buf := &bytes.Buffer{}
	assert.NoError(b, WriteHeaders(buf, headers))
	bs := buf.Bytes()
	reader := bytes.NewReader(bs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.Seek(0, 0)
		ReadHeaders(reader)
	}
}
