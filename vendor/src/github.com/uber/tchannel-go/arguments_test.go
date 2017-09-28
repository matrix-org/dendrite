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

package tchannel

import (
	"bytes"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type bufferWithClose struct {
	*bytes.Buffer
	closed bool
}

var _ io.WriteCloser = &bufferWithClose{}
var _ io.ReadCloser = &bufferWithClose{}

func newWriter() *bufferWithClose {
	return &bufferWithClose{bytes.NewBuffer(nil), false}
}

func newReader(bs []byte) *bufferWithClose {
	return &bufferWithClose{bytes.NewBuffer(bs), false}
}

func (w *bufferWithClose) Close() error {
	w.closed = true
	return nil
}

type testObject struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestJSONInputOutput(t *testing.T) {
	obj := testObject{Name: "Foo", Value: 20756}

	writer := newWriter()
	require.Nil(t, NewArgWriter(writer, nil).WriteJSON(obj))
	assert.True(t, writer.closed)
	assert.Equal(t, "{\"name\":\"Foo\",\"value\":20756}\n", writer.String())

	reader := newReader(writer.Bytes())
	outObj := testObject{}
	require.Nil(t, NewArgReader(reader, nil).ReadJSON(&outObj))

	assert.True(t, reader.closed)
	assert.Equal(t, "Foo", outObj.Name)
	assert.Equal(t, 20756, outObj.Value)
}

func TestReadNotEmpty(t *testing.T) {
	// Note: The contents need to be larger than the default buffer size of bufio.NewReader.
	r := bytes.NewReader([]byte("{}" + strings.Repeat("{}\n", 10000)))

	var data map[string]interface{}
	reader := NewArgReader(ioutil.NopCloser(r), nil)
	require.Error(t, reader.ReadJSON(&data), "Read should fail due to extra bytes")
}

func BenchmarkArgReaderWriter(b *testing.B) {
	obj := testObject{Name: "Foo", Value: 20756}
	outObj := testObject{}

	for i := 0; i < b.N; i++ {
		writer := newWriter()
		NewArgWriter(writer, nil).WriteJSON(obj)
		reader := newReader(writer.Bytes())
		NewArgReader(reader, nil).ReadJSON(&outObj)
	}

	b.StopTimer()
	assert.Equal(b, obj, outObj)
}
