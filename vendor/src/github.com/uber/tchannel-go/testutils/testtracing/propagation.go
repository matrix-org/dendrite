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
	"fmt"
	"testing"
	"time"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/testutils"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-client-go"
	"golang.org/x/net/context"
)

const (
	// BaggageKey is used for testing baggage propagation
	BaggageKey = "luggage"

	// BaggageValue is used for testing baggage propagation
	BaggageValue = "suitcase"
)

// TracingRequest tests tracing capabilities in a given server.
type TracingRequest struct {
	// ForwardCount tells the server how many times to forward this request to itself recursively
	ForwardCount int
}

// TracingResponse captures the trace info observed in the server and its downstream calls
type TracingResponse struct {
	TraceID        uint64
	SpanID         uint64
	ParentID       uint64
	TracingEnabled bool
	Child          *TracingResponse
	Luggage        string
}

// ObserveSpan extracts an OpenTracing span from the context and populates the response.
func (r *TracingResponse) ObserveSpan(ctx context.Context) *TracingResponse {
	if span := opentracing.SpanFromContext(ctx); span != nil {
		if mockSpan, ok := span.(*mocktracer.MockSpan); ok {
			sc := mockSpan.Context().(mocktracer.MockSpanContext)
			r.TraceID = uint64(sc.TraceID)
			r.SpanID = uint64(sc.SpanID)
			r.ParentID = uint64(mockSpan.ParentID)
			r.TracingEnabled = sc.Sampled
		} else if span := tchannel.CurrentSpan(ctx); span != nil {
			r.TraceID = span.TraceID()
			r.SpanID = span.SpanID()
			r.ParentID = span.ParentID()
			r.TracingEnabled = span.Flags()&1 == 1
		}
		r.Luggage = span.BaggageItem(BaggageKey)
	}
	return r
}

// TraceHandler is a base class for testing tracing propagation
type TraceHandler struct {
	Ch *tchannel.Channel
}

// HandleCall is used by handlers from different encodings as the main business logic.
// It respects the ForwardCount input parameter to make downstream calls, and returns
// a result containing the observed tracing span and the downstream results.
func (h *TraceHandler) HandleCall(
	ctx context.Context,
	req *TracingRequest,
	downstream TracingCall,
) (*TracingResponse, error) {
	var childResp *TracingResponse
	if req.ForwardCount > 0 {
		downstreamReq := &TracingRequest{ForwardCount: req.ForwardCount - 1}
		if resp, err := downstream(ctx, downstreamReq); err == nil {
			childResp = resp
		} else {
			return nil, err
		}
	}

	resp := &TracingResponse{Child: childResp}
	resp.ObserveSpan(ctx)

	return resp, nil
}

// TracerType is a convenient enum to indicate which type of tracer is being used in the test.
// It is a string because it's printed as part of the test description in the logs.
type TracerType string

const (
	// Noop is for the default no-op tracer from OpenTracing
	Noop TracerType = "NOOP"
	// Mock tracer, baggage-capable, non-Zipkin trace IDs
	Mock TracerType = "MOCK"
	// Jaeger is Uber's tracer, baggage-capable, Zipkin-style trace IDs
	Jaeger TracerType = "JAEGER"
)

// TracingCall is used in a few other structs here
type TracingCall func(ctx context.Context, req *TracingRequest) (*TracingResponse, error)

// EncodingInfo describes the encoding used with tracing propagation test
type EncodingInfo struct {
	Format           tchannel.Format
	HeadersSupported bool
}

// PropagationTestSuite is a collection of test cases for a certain encoding
type PropagationTestSuite struct {
	Encoding  EncodingInfo
	Register  func(t *testing.T, ch *tchannel.Channel) TracingCall
	TestCases map[TracerType][]PropagationTestCase
}

// PropagationTestCase describes a single propagation test case and expected results
type PropagationTestCase struct {
	ForwardCount      int
	TracingDisabled   bool
	ExpectedBaggage   string
	ExpectedSpanCount int
}

type tracerChoice struct {
	tracerType       TracerType
	tracer           opentracing.Tracer
	spansRecorded    func() int
	resetSpans       func()
	isFake           bool
	zipkinCompatible bool
}

// Run executes the test cases in the test suite against 3 different tracer implementations
func (s *PropagationTestSuite) Run(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"Noop_Tracer", s.runWithNoopTracer},
		{"Mock_Tracer", s.runWithMockTracer},
		{"Jaeger_Tracer", s.runWithJaegerTracer},
	}
	for _, test := range tests {
		t.Logf("Running with %s", test.name)
		test.run(t)
	}
}

func (s *PropagationTestSuite) runWithNoopTracer(t *testing.T) {
	s.runWithTracer(t, tracerChoice{
		tracer:        nil, // will cause opentracing.GlobalTracer() to be used
		tracerType:    Noop,
		spansRecorded: func() int { return 0 },
		resetSpans:    func() {},
		isFake:        true,
	})
}

func (s *PropagationTestSuite) runWithMockTracer(t *testing.T) {
	mockTracer := mocktracer.New()
	s.runWithTracer(t, tracerChoice{
		tracerType: Mock,
		tracer:     mockTracer,
		spansRecorded: func() int {
			return len(MockTracerSampledSpans(mockTracer))
		},
		resetSpans: func() {
			mockTracer.Reset()
		},
	})
}

func (s *PropagationTestSuite) runWithJaegerTracer(t *testing.T) {
	jaegerReporter := jaeger.NewInMemoryReporter()
	jaegerTracer, jaegerCloser := jaeger.NewTracer(testutils.DefaultServerName,
		jaeger.NewConstSampler(true),
		jaegerReporter)
	// To enable logging, use composite reporter:
	// jaeger.NewCompositeReporter(jaegerReporter, jaeger.NewLoggingReporter(jaeger.StdLogger)))
	defer jaegerCloser.Close()
	s.runWithTracer(t, tracerChoice{
		tracerType: Jaeger,
		tracer:     jaegerTracer,
		spansRecorded: func() int {
			return len(jaegerReporter.GetSpans())
		},
		resetSpans: func() {
			jaegerReporter.Reset()
		},
		zipkinCompatible: true,
	})
}

func (s *PropagationTestSuite) runWithTracer(t *testing.T, tracer tracerChoice) {
	testCases, ok := s.TestCases[tracer.tracerType]
	if !ok {
		t.Logf("No test cases for encoding=%s and tracer=%s", s.Encoding.Format, tracer.tracerType)
		return
	}
	opts := &testutils.ChannelOpts{
		ChannelOptions: tchannel.ChannelOptions{Tracer: tracer.tracer},
		DisableRelay:   true,
	}
	ch := testutils.NewServer(t, opts)
	defer ch.Close()
	ch.Peers().Add(ch.PeerInfo().HostPort)
	call := s.Register(t, ch)
	for _, tt := range testCases {
		s.runTestCase(t, tracer, ch, tt, call)
	}
}

func (s *PropagationTestSuite) runTestCase(
	t *testing.T,
	tracer tracerChoice,
	ch *tchannel.Channel,
	test PropagationTestCase,
	call TracingCall,
) {
	descr := fmt.Sprintf("test %+v with tracer %+v", test, tracer)
	ch.Logger().Debugf("Starting tracing test %s", descr)

	tracer.resetSpans()

	span := ch.Tracer().StartSpan("client")
	span.SetBaggageItem(BaggageKey, BaggageValue)
	ctx := opentracing.ContextWithSpan(context.Background(), span)

	ctxBuilder := tchannel.NewContextBuilder(5 * time.Second).SetParentContext(ctx)
	if test.TracingDisabled {
		ctxBuilder.DisableTracing()
	}
	ctx, cancel := ctxBuilder.Build()
	defer cancel()

	req := &TracingRequest{ForwardCount: test.ForwardCount}
	ch.Logger().Infof("Sending tracing request %+v", req)
	response, err := call(ctx, req)
	require.NoError(t, err)
	ch.Logger().Infof("Received tracing response %+v", response)

	// Spans are finished in inbound.doneSending() or outbound.doneReading(),
	// which are called on different go-routines and may execute *after* the
	// response has been received by the client. Give them a chance to run.
	for i := 0; i < 1000; i++ {
		if spanCount := tracer.spansRecorded(); spanCount == test.ExpectedSpanCount {
			break
		}
		time.Sleep(testutils.Timeout(time.Millisecond))
	}
	spanCount := tracer.spansRecorded()
	ch.Logger().Debugf("end span count: %d", spanCount)

	// finish span after taking count of recorded spans, as we're only interested
	// in the count of spans created by RPC calls.
	span.Finish()

	root := new(TracingResponse).ObserveSpan(ctx)

	if !tracer.isFake {
		assert.Equal(t, uint64(0), root.ParentID)
		assert.NotEqual(t, uint64(0), root.TraceID)
	}

	assert.Equal(t, test.ExpectedSpanCount, spanCount, "Wrong span count; %s", descr)

	for r, cnt := response, 0; r != nil || cnt <= test.ForwardCount; r, cnt = r.Child, cnt+1 {
		require.NotNil(t, r, "Expecting response for forward=%d; %s", cnt, descr)
		if !tracer.isFake {
			if tracer.zipkinCompatible || s.Encoding.HeadersSupported {
				assert.Equal(t, root.TraceID, r.TraceID, "traceID should be the same; %s", descr)
			}
			assert.Equal(t, test.ExpectedBaggage, r.Luggage, "baggage should propagate; %s", descr)
		}
	}
	ch.Logger().Debugf("Finished tracing test %s", descr)
}

// MockTracerSampledSpans is a helper function that returns only sampled spans from MockTracer
func MockTracerSampledSpans(tracer *mocktracer.MockTracer) []*mocktracer.MockSpan {
	var spans []*mocktracer.MockSpan
	for _, span := range tracer.FinishedSpans() {
		if span.Context().(mocktracer.MockSpanContext).Sampled {
			spans = append(spans, span)
		}
	}
	return spans
}
