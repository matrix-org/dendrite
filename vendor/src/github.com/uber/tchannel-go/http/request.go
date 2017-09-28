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

package http

import (
	"io"
	"net/http"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/typed"
)

// WriteRequest writes a http.Request to the given writers.
func WriteRequest(call tchannel.ArgWritable, req *http.Request) error {
	// TODO(prashant): Allow creating write buffers that let you grow the buffer underneath.
	wb := typed.NewWriteBufferWithSize(10000)
	wb.WriteLen8String(req.Method)
	writeVarintString(wb, req.URL.String())
	writeHeaders(wb, req.Header)

	arg2Writer, err := call.Arg2Writer()
	if err != nil {
		return err
	}
	if _, err := wb.FlushTo(arg2Writer); err != nil {
		return err
	}
	if err := arg2Writer.Close(); err != nil {
		return err
	}

	arg3Writer, err := call.Arg3Writer()
	if err != nil {
		return err
	}

	if req.Body != nil {
		if _, err = io.Copy(arg3Writer, req.Body); err != nil {
			return err
		}
	}
	return arg3Writer.Close()
}

// ReadRequest reads a http.Request from the given readers.
func ReadRequest(call tchannel.ArgReadable) (*http.Request, error) {
	var arg2 []byte
	if err := tchannel.NewArgReader(call.Arg2Reader()).Read(&arg2); err != nil {
		return nil, err
	}
	rb := typed.NewReadBuffer(arg2)
	method := rb.ReadLen8String()
	url := readVarintString(rb)

	r, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	readHeaders(rb, r.Header)

	if err := rb.Err(); err != nil {
		return nil, err
	}

	r.Body, err = call.Arg3Reader()
	return r, err
}
