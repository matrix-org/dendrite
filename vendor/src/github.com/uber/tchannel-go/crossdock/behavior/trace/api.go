// Copyright (c) 2016 Uber Technologies, Inc.
//
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

package trace

// Request instructs the server to call another server recursively if Downstream != nil,
// and return the results of the downstream call as well as the current tracing span it
// observes in its Context.
type Request struct {
	ServerRole string      `json:"serverRole"`
	Downstream *Downstream `json:"downstream,omitempty"`
}

// Downstream describes which downstream service to call recursively.
type Downstream struct {
	ServiceName string      `json:"serviceName"`
	ServerRole  string      `json:"serverRole"`
	Encoding    string      `json:"encoding"`
	HostPort    string      `json:"hostPort"`
	Downstream  *Downstream `json:"downstream,omitempty"`
}

// Response contains the span observed by the server and nested downstream response.
type Response struct {
	Span       *ObservedSpan `json:"span,omitempty"`
	Downstream *Response     `json:"downstream,omitempty"`
}

// ObservedSpan describes the tracing span observed by the server
type ObservedSpan struct {
	TraceID string `json:"traceId"`
	Sampled bool   `json:"sampled"`
	Baggage string `json:"baggage"`
}
