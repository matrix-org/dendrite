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

package thrift_test

import (
	"bytes"
	"io/ioutil"
	"sync"
	"testing"

	. "github.com/uber/tchannel-go/thrift"

	"github.com/uber/tchannel-go/testutils/testreader"
	"github.com/uber/tchannel-go/testutils/testwriter"
	"github.com/uber/tchannel-go/thrift/gen-go/test"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
)

var structTest = struct {
	s       thrift.TStruct
	encoded []byte
}{
	s: &test.Data{
		B1: true,
		S2: "S2",
		I3: 3,
	},
	encoded: []byte{
		0x2,      // bool
		0x0, 0x1, // field 1
		0x1,      // true
		0xb,      // string
		0x0, 0x2, // field 2
		0x0, 0x0, 0x0, 0x2, // length of string "S2"
		'S', '2', // string "S2"
		0x8,      // i32
		0x0, 0x3, // field 3
		0x0, 0x0, 0x0, 0x3, // i32 3
		0x0, // end of struct
	},
}

func TestReadStruct(t *testing.T) {
	appendBytes := func(bs []byte, append []byte) []byte {
		b := make([]byte, len(bs)+len(append))
		n := copy(b, bs)
		copy(b[n:], append)
		return b
	}

	tests := []struct {
		s        thrift.TStruct
		encoded  []byte
		wantErr  bool
		leftover []byte
	}{
		{
			s:       structTest.s,
			encoded: structTest.encoded,
		},
		{
			s: &test.Data{
				B1: true,
				S2: "S2",
			},
			// Missing field 3.
			encoded: structTest.encoded[:len(structTest.encoded)-8],
			wantErr: true,
		},
		{
			s:        structTest.s,
			encoded:  appendBytes(structTest.encoded, []byte{1, 2, 3, 4}),
			leftover: []byte{1, 2, 3, 4},
		},
	}

	for _, tt := range tests {
		reader := bytes.NewReader(tt.encoded)
		var s thrift.TStruct = &test.Data{}
		err := ReadStruct(reader, s)
		assert.Equal(t, tt.wantErr, err != nil, "Unexpected error: %v", err)

		// Even if there's an error, the struct will be partially filled.
		assert.Equal(t, tt.s, s, "Unexpected struct")

		leftover, err := ioutil.ReadAll(reader)
		if assert.NoError(t, err, "Read leftover bytes failed") {
			// ReadAll always returns a non-nil byte slice.
			if tt.leftover == nil {
				tt.leftover = make([]byte, 0)
			}
			assert.Equal(t, tt.leftover, leftover, "Leftover bytes mismatch")
		}
	}
}

func TestReadStructErr(t *testing.T) {
	writer, reader := testreader.ChunkReader()
	writer <- structTest.encoded[:10]
	writer <- nil
	close(writer)

	s := &test.Data{}
	err := ReadStruct(reader, s)
	if assert.Error(t, err, "ReadStruct should fail") {
		// Apache Thrift just prepends the error message, and doesn't give us access
		// to the underlying error, so we can't check the underlying error exactly.
		assert.Contains(t, err.Error(), testreader.ErrUser.Error(), "Underlying error missing")
	}
}

func TestWriteStruct(t *testing.T) {
	tests := []struct {
		s       thrift.TStruct
		encoded []byte
		wantErr bool
	}{
		{
			s:       structTest.s,
			encoded: structTest.encoded,
		},
	}

	for _, tt := range tests {
		buf := &bytes.Buffer{}
		err := WriteStruct(buf, tt.s)
		assert.Equal(t, tt.wantErr, err != nil, "Unexpected err: %v", err)
		if err != nil {
			continue
		}

		assert.Equal(t, tt.encoded, buf.Bytes(), "Encoded data mismatch")
	}
}

func TestWriteStructErr(t *testing.T) {
	writer := testwriter.Limited(10)
	err := WriteStruct(writer, structTest.s)
	if assert.Error(t, err, "WriteStruct should fail") {
		// Apache Thrift just prepends the error message, and doesn't give us access
		// to the underlying error, so we can't check the underlying error exactly.
		assert.Contains(t, err.Error(), testwriter.ErrOutOfSpace.Error(), "Underlying error missing")
	}
}

func TestParallelReadWrites(t *testing.T) {
	var wg sync.WaitGroup
	testBG := func(f func(t *testing.T)) {
		wg.Add(1)
		go func() {
			f(t)
			wg.Done()
		}()
	}
	for i := 0; i < 50; i++ {
		testBG(TestReadStruct)
		testBG(TestWriteStruct)
	}
	wg.Wait()
}

func BenchmarkWriteStruct(b *testing.B) {
	buf := &bytes.Buffer{}
	for i := 0; i < b.N; i++ {
		buf.Reset()
		WriteStruct(buf, structTest.s)
	}
}

func BenchmarkReadStruct(b *testing.B) {
	buf := bytes.NewReader(structTest.encoded)
	var d test.Data

	buf.Seek(0, 0)
	assert.NoError(b, ReadStruct(buf, &d))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Seek(0, 0)
		ReadStruct(buf, &d)
	}
}
