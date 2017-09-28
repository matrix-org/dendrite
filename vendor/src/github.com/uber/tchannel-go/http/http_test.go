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
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"testing"
	"time"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func dumpHandler(w http.ResponseWriter, r *http.Request) {
	r.URL.Host = "test.local"
	r.URL.Scheme = "http"

	// We cannot use httputil.DumpRequestOut as it prints the chunked encoding
	// while we only care about the data that the reader would see.
	dump := &bytes.Buffer{}
	dump.WriteString(r.Method)
	dump.WriteString(r.URL.String())
	dump.WriteString("\n")

	dump.WriteString("Headers: ")
	dump.WriteString(fmt.Sprint(r.Form))
	dump.WriteString("\n")

	dump.WriteString("Body: ")
	io.Copy(dump, r.Body)
	dump.WriteString("\n")

	w.Header().Add("My-Header-1", "V1")
	w.Header().Add("My-Header-1", "V2")
	w.Header().Add("My-Header-2", "V3")

	w.Write([]byte("Dumped request:\n"))
	w.Write(dump.Bytes())
}

func setupHTTP(t *testing.T, serveMux *http.ServeMux) (string, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "net.Listen failed")

	go http.Serve(ln, serveMux)
	httpAddr := ln.Addr().String()
	return httpAddr, func() { ln.Close() }
}

func setupTChan(t *testing.T, mux *http.ServeMux) (string, func()) {
	ch := testutils.NewServer(t, testutils.NewOpts().SetServiceName("test"))
	handler := func(ctx context.Context, call *tchannel.InboundCall) {
		req, err := ReadRequest(call)
		if !assert.NoError(t, err, "ReadRequest failed") {
			return
		}

		// Make the HTTP call using the default mux.
		writer, finish := ResponseWriter(call.Response())
		mux.ServeHTTP(writer, req)
		finish()
	}
	ch.Register(tchannel.HandlerFunc(handler), "http")
	return ch.PeerInfo().HostPort, func() { ch.Close() }
}

func setupProxy(t *testing.T, tchanAddr string) (string, func()) {
	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// You get /proxy/host:port/rest/of/the/path
		parts := strings.SplitN(r.URL.Path, "/", 4)
		r.URL.Host = parts[2]
		r.URL.Scheme = "http"
		r.URL.Path = parts[3]

		ch := testutils.NewClient(t, nil)
		ctx, cancel := tchannel.NewContext(time.Second)
		defer cancel()

		call, err := ch.BeginCall(ctx, tchanAddr, "test", "http", nil)
		require.NoError(t, err, "BeginCall failed")

		require.NoError(t, WriteRequest(call, r), "WriteRequest failed")
		resp, err := ReadResponse(call.Response())
		require.NoError(t, err, "Read response failed")

		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)

		_, err = io.Copy(w, resp.Body)
		assert.NoError(t, err, "io.Copy failed")
		err = resp.Body.Close()
		assert.NoError(t, err, "Close Response Body failed")
	}))
	return setupHTTP(t, mux)
}

// setupServer sets up a HTTP handler and a TChannel handler .
func setupServer(t *testing.T) (string, string, func()) {
	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(dumpHandler))

	httpAddr, httpClose := setupHTTP(t, mux)
	tchanAddr, tchanClose := setupTChan(t, mux)

	close := func() {
		httpClose()
		tchanClose()
	}
	return httpAddr, tchanAddr, close
}

func makeHTTPCall(t *testing.T, req *http.Request) *http.Response {
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "HTTP request failed")
	return resp
}

func makeTChanCall(t *testing.T, tchanAddr string, req *http.Request) *http.Response {
	ch := testutils.NewClient(t, nil)
	ctx, cancel := tchannel.NewContext(time.Second)
	defer cancel()

	call, err := ch.BeginCall(ctx, tchanAddr, "test", "http", nil)
	require.NoError(t, err, "BeginCall failed")

	require.NoError(t, WriteRequest(call, req), "WriteRequest failed")
	resp, err := ReadResponse(call.Response())
	require.NoError(t, err, "Read response failed")

	return resp
}

func compareResponseBasic(t *testing.T, testName string, resp1, resp2 *http.Response) {
	resp1Body, err := ioutil.ReadAll(resp1.Body)
	require.NoError(t, err, "Read response failed")
	resp2Body, err := ioutil.ReadAll(resp2.Body)
	require.NoError(t, err, "Read response failed")

	assert.Equal(t, resp1.Status, resp2.Status, "%v: Response status mismatch", testName)
	assert.Equal(t, resp1.StatusCode, resp2.StatusCode, "%v: Response status code mismatch", testName)
	assert.Equal(t, string(resp1Body), string(resp2Body), "%v: Response body mismatch", testName)
}

func compareResponses(t *testing.T, testName string, resp1, resp2 *http.Response) {
	resp1Bs, err := httputil.DumpResponse(resp1, true)
	require.NoError(t, err, "Dump response")
	resp2Bs, err := httputil.DumpResponse(resp2, true)
	require.NoError(t, err, "Dump response")
	assert.Equal(t, string(resp1Bs), string(resp2Bs), "%v: Response mismatch", testName)
}

type requestTest struct {
	name string
	f    func(string) *http.Request
}

func getRequestTests(t *testing.T) []requestTest {
	randBytes := testutils.RandBytes(40000)
	return []requestTest{
		{
			name: "get simple",
			f: func(httpAddr string) *http.Request {
				req, err := http.NewRequest("GET", fmt.Sprintf("http://%v/this/is/my?req=1&v=2&v&a&a", httpAddr), nil)
				require.NoError(t, err, "NewRequest failed")
				return req
			},
		},
		{
			name: "post simple",
			f: func(httpAddr string) *http.Request {
				body := strings.NewReader("This is a simple POST body")
				req, err := http.NewRequest("POST", fmt.Sprintf("http://%v/post/path?v=1&b=3", httpAddr), body)
				require.NoError(t, err, "NewRequest failed")
				return req
			},
		},
		{
			name: "post random bytes",
			f: func(httpAddr string) *http.Request {
				body := bytes.NewReader(randBytes)
				req, err := http.NewRequest("POST", fmt.Sprintf("http://%v/post/path?v=1&b=3", httpAddr), body)
				require.NoError(t, err, "NewRequest failed")
				return req
			},
		},
	}
}

func TestDirectRequests(t *testing.T) {
	httpAddr, tchanAddr, finish := setupServer(t)
	defer finish()

	tests := getRequestTests(t)
	for _, tt := range tests {
		resp1 := makeHTTPCall(t, tt.f(httpAddr))
		resp2 := makeTChanCall(t, tchanAddr, tt.f(httpAddr))
		compareResponseBasic(t, tt.name, resp1, resp2)
	}
}

func TestProxyRequests(t *testing.T) {
	httpAddr, tchanAddr, finish := setupServer(t)
	defer finish()
	proxyAddr, finish := setupProxy(t, tchanAddr)
	defer finish()

	tests := getRequestTests(t)
	for _, tt := range tests {
		resp1 := makeHTTPCall(t, tt.f(httpAddr))
		resp2 := makeHTTPCall(t, tt.f(proxyAddr+"/proxy/"+httpAddr))

		// Delete the Date header since the calls are made at different times.
		resp1.Header.Del("Date")
		resp2.Header.Del("Date")
		compareResponses(t, tt.name, resp1, resp2)
	}
}
