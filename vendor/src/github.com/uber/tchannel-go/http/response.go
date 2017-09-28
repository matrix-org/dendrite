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
	"fmt"
	"io"
	"net/http"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/typed"
)

// ReadResponse reads a http.Response from the given readers.
func ReadResponse(call tchannel.ArgReadable) (*http.Response, error) {
	var arg2 []byte
	if err := tchannel.NewArgReader(call.Arg2Reader()).Read(&arg2); err != nil {
		return nil, err
	}

	rb := typed.NewReadBuffer(arg2)
	statusCode := rb.ReadUint16()
	message := readVarintString(rb)

	response := &http.Response{
		StatusCode: int(statusCode),
		Status:     fmt.Sprintf("%v %v", statusCode, message),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
	}
	readHeaders(rb, response.Header)
	if err := rb.Err(); err != nil {
		return nil, err
	}

	arg3Reader, err := call.Arg3Reader()
	if err != nil {
		return nil, err
	}

	response.Body = arg3Reader
	return response, nil
}

type tchanResponseWriter struct {
	headers    http.Header
	statusCode int
	response   tchannel.ArgWritable
	arg3Writer io.WriteCloser
	err        error
}

func newTChanResponseWriter(response tchannel.ArgWritable) *tchanResponseWriter {
	return &tchanResponseWriter{
		headers:    make(http.Header),
		statusCode: http.StatusOK,
		response:   response,
	}
}

func (w *tchanResponseWriter) Header() http.Header {
	return w.headers
}

func (w *tchanResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

// writeHeaders writes out the HTTP headers as arg2, and creates the arg3 writer.
func (w *tchanResponseWriter) writeHeaders() {
	// TODO(prashant): Allow creating write buffers that let you grow the buffer underneath.
	wb := typed.NewWriteBufferWithSize(10000)
	wb.WriteUint16(uint16(w.statusCode))
	writeVarintString(wb, http.StatusText(w.statusCode))
	writeHeaders(wb, w.headers)

	arg2Writer, err := w.response.Arg2Writer()
	if err != nil {
		w.err = err
		return
	}
	if _, w.err = wb.FlushTo(arg2Writer); w.err != nil {
		return
	}
	if w.err = arg2Writer.Close(); w.err != nil {
		return
	}

	w.arg3Writer, w.err = w.response.Arg3Writer()
}

func (w *tchanResponseWriter) Write(bs []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}

	if w.arg3Writer == nil {
		w.writeHeaders()
	}
	if w.err != nil {
		return 0, w.err
	}

	return w.arg3Writer.Write(bs)
}

func (w *tchanResponseWriter) finish() error {
	if w.arg3Writer == nil || w.err != nil {
		return w.err
	}
	return w.arg3Writer.Close()
}

// ResponseWriter returns a http.ResponseWriter that will write to an underlying writer.
// It also returns a function that should be called once the handler has completed.
func ResponseWriter(response tchannel.ArgWritable) (http.ResponseWriter, func() error) {
	responseWriter := newTChanResponseWriter(response)
	return responseWriter, responseWriter.finish
}
