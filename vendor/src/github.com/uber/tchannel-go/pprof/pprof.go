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

package pprof

import (
	"net/http"
	_ "net/http/pprof" // So pprof endpoints are registered on DefaultServeMux.

	"github.com/uber/tchannel-go"
	thttp "github.com/uber/tchannel-go/http"

	"golang.org/x/net/context"
)

func serveHTTP(req *http.Request, response *tchannel.InboundCallResponse) {
	rw, finish := thttp.ResponseWriter(response)
	http.DefaultServeMux.ServeHTTP(rw, req)
	finish()
}

// Register registers pprof endpoints on the given registrar under _pprof.
// The _pprof endpoint uses as-http and is a tunnel to the default serve mux.
func Register(registrar tchannel.Registrar) {
	handler := func(ctx context.Context, call *tchannel.InboundCall) {
		req, err := thttp.ReadRequest(call)
		if err != nil {
			registrar.Logger().WithFields(
				tchannel.LogField{Key: "err", Value: err.Error()},
			).Warn("Failed to read HTTP request.")
			return
		}

		serveHTTP(req, call.Response())
	}
	registrar.Register(tchannel.HandlerFunc(handler), "_pprof")
}
