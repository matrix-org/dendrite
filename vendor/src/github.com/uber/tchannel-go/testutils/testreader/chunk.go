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
	"errors"
	"io"
)

// ErrUser is returned by ChunkReader when the user requests an error.
var ErrUser = errors.New("error set by user")

// ChunkReader returns a reader that returns chunks written to the control channel.
// The caller should write byte chunks to return to the channel, or write nil if they
// want the Reader to return an error. The control channel should be closed to signal EOF.
func ChunkReader() (chan<- []byte, io.Reader) {
	reader := &errorReader{
		c: make(chan []byte, 100),
	}
	return reader.c, reader
}

type errorReader struct {
	c         chan []byte
	remaining []byte
}

func (r *errorReader) Read(bs []byte) (int, error) {
	for len(r.remaining) == 0 {
		var ok bool
		r.remaining, ok = <-r.c
		if !ok {
			return 0, io.EOF
		}
		if r.remaining == nil {
			return 0, ErrUser
		}
		if len(r.remaining) == 0 {
			return 0, nil
		}
	}

	n := copy(bs, r.remaining)
	r.remaining = r.remaining[n:]
	return n, nil
}
