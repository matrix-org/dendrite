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

package tchannel_test

import (
	"sync"
	"testing"
	"time"

	. "github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/json"
	"github.com/uber/tchannel-go/testutils"
	"github.com/uber/tchannel-go/testutils/testtracing"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

// JSONHandler tests tracing over JSON encoding
type JSONHandler struct {
	testtracing.TraceHandler
	t *testing.T
}

func (h *JSONHandler) callJSON(ctx json.Context, req *testtracing.TracingRequest) (*testtracing.TracingResponse, error) {
	resp := new(testtracing.TracingResponse)
	resp.ObserveSpan(ctx)
	return resp, nil
}

func (h *JSONHandler) onError(ctx context.Context, err error) { h.t.Errorf("onError %v", err) }

func TestTracingSpanAttributes(t *testing.T) {
	tracer := mocktracer.New()

	opts := &testutils.ChannelOpts{
		ChannelOptions: ChannelOptions{Tracer: tracer},
		DisableRelay:   true,
	}
	WithVerifiedServer(t, opts, func(ch *Channel, hostPort string) {
		// Register JSON handler
		jsonHandler := &JSONHandler{TraceHandler: testtracing.TraceHandler{Ch: ch}, t: t}
		json.Register(ch, json.Handlers{"call": jsonHandler.callJSON}, jsonHandler.onError)

		span := ch.Tracer().StartSpan("client")
		span.SetBaggageItem(testtracing.BaggageKey, testtracing.BaggageValue)
		ctx := opentracing.ContextWithSpan(context.Background(), span)
		root := new(testtracing.TracingResponse).ObserveSpan(ctx)

		ctx, cancel := NewContextBuilder(2 * time.Second).SetParentContext(ctx).Build()
		defer cancel()

		peer := ch.Peers().GetOrAdd(ch.PeerInfo().HostPort)
		var response testtracing.TracingResponse
		require.NoError(t, json.CallPeer(json.Wrap(ctx), peer, ch.PeerInfo().ServiceName,
			"call", &testtracing.TracingRequest{}, &response))

		// Spans are finished in inbound.doneSending() or outbound.doneReading(),
		// which are called on different go-routines and may execute *after* the
		// response has been received by the client. Give them a chance to finish.
		for i := 0; i < 1000; i++ {
			if spanCount := len(testtracing.MockTracerSampledSpans(tracer)); spanCount == 2 {
				break
			}
			time.Sleep(testutils.Timeout(time.Millisecond))
		}
		spans := testtracing.MockTracerSampledSpans(tracer)
		spanCount := len(spans)
		ch.Logger().Debugf("end span count: %d", spanCount)

		// finish span after taking count of recorded spans
		span.Finish()

		require.Equal(t, 2, spanCount, "Wrong span count")
		assert.Equal(t, root.TraceID, response.TraceID, "Trace ID must match root span")
		assert.Equal(t, testtracing.BaggageValue, response.Luggage, "Baggage must match")

		var parent, child *mocktracer.MockSpan
		for _, s := range spans {
			if s.Tag("span.kind") == ext.SpanKindRPCClientEnum {
				parent = s
				ch.Logger().Debugf("Found parent span: %+v", s)
			} else if s.Tag("span.kind") == ext.SpanKindRPCServerEnum {
				child = s
				ch.Logger().Debugf("Found child span: %+v", s)
			}
		}

		require.NotNil(t, parent)
		require.NotNil(t, child)

		traceID := func(s opentracing.Span) int {
			return s.Context().(mocktracer.MockSpanContext).TraceID
		}
		spanID := func(s *mocktracer.MockSpan) int {
			return s.Context().(mocktracer.MockSpanContext).SpanID
		}
		sampled := func(s *mocktracer.MockSpan) bool {
			return s.Context().(mocktracer.MockSpanContext).Sampled
		}

		require.Equal(t, traceID(span), traceID(parent), "parent must be found")
		require.Equal(t, traceID(span), traceID(child), "child must be found")
		assert.Equal(t, traceID(parent), traceID(child))
		assert.Equal(t, spanID(parent), child.ParentID)
		assert.True(t, sampled(parent), "should be sampled")
		assert.True(t, sampled(child), "should be sampled")
		assert.Equal(t, "call", parent.OperationName)
		assert.Equal(t, "call", child.OperationName)
		assert.Equal(t, "testService", parent.Tag("peer.service"))
		assert.Equal(t, "testService", child.Tag("peer.service"))
		assert.Equal(t, "json", parent.Tag("as"))
		assert.Equal(t, "json", child.Tag("as"))
		assert.NotNil(t, parent.Tag("peer.ipv4"))
		assert.NotNil(t, child.Tag("peer.ipv4"))
		assert.NotNil(t, parent.Tag("peer.port"))
		assert.NotNil(t, child.Tag("peer.port"))
	})
}

// Per https://github.com/uber/tchannel-go/issues/505, concurrent client calls
// made with the same shared map used as headers were causing panic due to
// concurrent writes to the map when injecting tracing headers.
func TestReusableHeaders(t *testing.T) {
	opts := &testutils.ChannelOpts{
		ChannelOptions: ChannelOptions{Tracer: mocktracer.New()},
	}
	WithVerifiedServer(t, opts, func(ch *Channel, hostPort string) {
		jsonHandler := &JSONHandler{TraceHandler: testtracing.TraceHandler{Ch: ch}, t: t}
		json.Register(ch, json.Handlers{"call": jsonHandler.callJSON}, jsonHandler.onError)

		span := ch.Tracer().StartSpan("client")
		traceID := span.(*mocktracer.MockSpan).SpanContext.TraceID // for validation
		ctx := opentracing.ContextWithSpan(context.Background(), span)

		sharedHeaders := map[string]string{"life": "42"}
		ctx, cancel := NewContextBuilder(2 * time.Second).
			SetHeaders(sharedHeaders).
			SetParentContext(ctx).
			Build()
		defer cancel()

		peer := ch.Peers().GetOrAdd(ch.PeerInfo().HostPort)

		var wg sync.WaitGroup
		for i := 0; i < 42; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var response testtracing.TracingResponse
				err := json.CallPeer(json.Wrap(ctx), peer, ch.ServiceName(),
					"call", &testtracing.TracingRequest{}, &response)
				assert.NoError(t, err, "json.Call failed")
				assert.EqualValues(t, traceID, response.TraceID, "traceID must match")
			}()
		}
		wg.Wait()
		assert.Equal(t, map[string]string{"life": "42"}, sharedHeaders, "headers unchanged")
	})
}
