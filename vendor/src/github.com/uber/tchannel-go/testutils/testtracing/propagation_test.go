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

package testtracing

import (
	json_encoding "encoding/json"
	"testing"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"

	"golang.org/x/net/context"
)

func requestFromRaw(args *raw.Args) *TracingRequest {
	r := new(TracingRequest)
	r.ForwardCount = int(args.Arg3[0])
	return r
}

func requestToRaw(r *TracingRequest) []byte {
	return []byte{byte(r.ForwardCount)}
}

func responseFromRaw(t *testing.T, arg3 []byte) (*TracingResponse, error) {
	var r TracingResponse
	err := json_encoding.Unmarshal(arg3, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func responseToRaw(t *testing.T, r *TracingResponse) (*raw.Res, error) {
	jsonBytes, err := json_encoding.Marshal(r)
	if err != nil {
		return nil, err
	}
	return &raw.Res{Arg3: jsonBytes}, nil
}

// RawHandler tests tracing over Raw encoding
type RawHandler struct {
	TraceHandler
	t *testing.T
}

func (h *RawHandler) Handle(ctx context.Context, args *raw.Args) (*raw.Res, error) {
	req := requestFromRaw(args)
	res, err := h.HandleCall(ctx, req,
		func(ctx context.Context, req *TracingRequest) (*TracingResponse, error) {
			_, arg3, _, err := raw.Call(ctx, h.Ch, h.Ch.PeerInfo().HostPort,
				h.Ch.PeerInfo().ServiceName, "rawcall", nil, requestToRaw(req))
			if err != nil {
				return nil, err
			}
			return responseFromRaw(h.t, arg3)
		})
	if err != nil {
		return nil, err
	}
	return responseToRaw(h.t, res)
}

func (h *RawHandler) OnError(ctx context.Context, err error) { h.t.Errorf("onError %v", err) }

func (h *RawHandler) firstCall(ctx context.Context, req *TracingRequest) (*TracingResponse, error) {
	_, arg3, _, err := raw.Call(ctx, h.Ch, h.Ch.PeerInfo().HostPort, h.Ch.PeerInfo().ServiceName,
		"rawcall", nil, requestToRaw(req))
	if err != nil {
		return nil, err
	}
	return responseFromRaw(h.t, arg3)
}

func TestRawTracingPropagation(t *testing.T) {
	suite := &PropagationTestSuite{
		Encoding: EncodingInfo{Format: tchannel.Raw, HeadersSupported: false},
		Register: func(t *testing.T, ch *tchannel.Channel) TracingCall {
			handler := &RawHandler{
				TraceHandler: TraceHandler{Ch: ch},
				t:            t,
			}
			ch.Register(raw.Wrap(handler), "rawcall")
			return handler.firstCall
		},
		// Since Raw encoding does not support headers, there is no baggage propagation
		TestCases: map[TracerType][]PropagationTestCase{
			Noop: {
				{ForwardCount: 2, TracingDisabled: true, ExpectedBaggage: "", ExpectedSpanCount: 0},
				{ForwardCount: 2, TracingDisabled: false, ExpectedBaggage: "", ExpectedSpanCount: 0},
			},
			Mock: {
				// Since Raw encoding does not propagate generic traces, the tracingDisable
				// only affects the first outbound span (it's not sampled), but the other
				// two outbound spans are still sampled and recorded.
				{ForwardCount: 2, TracingDisabled: true, ExpectedBaggage: "", ExpectedSpanCount: 2},
				// Since Raw encoding does not propagate generic traces, we record 3 spans
				// for outbound calls, but none for inbound calls.
				{ForwardCount: 2, TracingDisabled: false, ExpectedBaggage: "", ExpectedSpanCount: 3},
			},
			Jaeger: {
				// Since Jaeger is Zipkin-compatible, it is able to keep track of tracingDisabled
				{ForwardCount: 2, TracingDisabled: true, ExpectedBaggage: "", ExpectedSpanCount: 0},
				// Since Jaeger is Zipkin-compatible, it is able to decode the trace
				// even from the Raw encoding.
				{ForwardCount: 2, TracingDisabled: false, ExpectedBaggage: "", ExpectedSpanCount: 6},
			},
		},
	}
	suite.Run(t)
}
