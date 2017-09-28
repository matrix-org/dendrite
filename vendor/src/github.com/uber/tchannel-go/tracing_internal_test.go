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
	"fmt"
	"net"
	"testing"

	"github.com/uber/tchannel-go/typed"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func TestTracingSpanEncoding(t *testing.T) {
	s1 := Span{
		traceID:  1,
		parentID: 2,
		spanID:   3,
		flags:    4,
	}
	// Encoding is: spanid:8 parentid:8 traceid:8 traceflags:1
	// http://tchannel.readthedocs.io/en/latest/protocol/#tracing
	encoded := []byte{
		0, 0, 0, 0, 0, 0, 0, 3, /* spanID */
		0, 0, 0, 0, 0, 0, 0, 2, /* parentID */
		0, 0, 0, 0, 0, 0, 0, 1, /* traceID */
		4, /* flags */
	}

	buf := make([]byte, len(encoded))
	writer := typed.NewWriteBuffer(buf)
	require.NoError(t, s1.write(writer), "Failed to encode span")

	assert.Equal(t, encoded, buf, "Encoded span mismatch")

	var s2 Span
	reader := typed.NewReadBuffer(buf)
	require.NoError(t, s2.read(reader), "Failed to decode span")

	assert.Equal(t, s1, s2, "Roundtrip of span failed")
}

func TestTracingInjectorExtractor(t *testing.T) {
	tracer := mocktracer.New()
	tracer.RegisterInjector(zipkinSpanFormat, new(zipkinInjector))
	tracer.RegisterExtractor(zipkinSpanFormat, new(zipkinExtractor))

	sp := tracer.StartSpan("x")
	var injectable injectableSpan
	err := tracer.Inject(sp.Context(), zipkinSpanFormat, &injectable)
	require.NoError(t, err)

	tsp := Span(injectable)
	assert.NotEqual(t, uint64(0), tsp.TraceID())
	assert.NotEqual(t, uint64(0), tsp.SpanID())

	sp2, err := tracer.Extract(zipkinSpanFormat, &tsp)
	require.NoError(t, err)
	require.NotNil(t, sp2)
}

func TestSpanString(t *testing.T) {
	span := Span{traceID: 15}
	assert.Equal(t, "TraceID=f,ParentID=0,SpanID=0", span.String())
}

func TestSetPeerHostPort(t *testing.T) {
	tracer := mocktracer.New()

	ipv6 := []byte{1, 2, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 15, 16}
	assert.Equal(t, net.IPv6len, len(ipv6))
	ipv6hostPort := fmt.Sprintf("[%v]:789", net.IP(ipv6))

	tests := []struct {
		hostPort    string
		wantHostTag string
		wantHost    interface{}
		wantPort    uint16
	}{
		{"adhoc123:bad-port", "peer.hostname", "adhoc123", 0},
		{"adhoc123", "peer.hostname", "adhoc123", 0},
		{"ip123.uswest.aws.com:765", "peer.hostname", "ip123.uswest.aws.com", 765},
		{"localhost:123", "peer.ipv4", uint32(127<<24 | 1), 123},
		{"10.20.30.40:321", "peer.ipv4", uint32(10<<24 | 20<<16 | 30<<8 | 40), 321},
		{ipv6hostPort, "peer.ipv6", "102:300::f10", 789},
	}

	for i, test := range tests {
		span := tracer.StartSpan("x")
		peerInfo, peerAddress, err := parseRemotePeer(initParams{
			InitParamHostPort:    test.hostPort,
			InitParamProcessName: "test",
		}, &net.IPAddr{IP: net.ParseIP("1.1.1.1")})
		require.NoError(t, err, "Failed to parse remote peer info")

		c := &Connection{
			channelConnectionCommon: channelConnectionCommon{
				log: NullLogger,
			},
			remotePeerInfo:    peerInfo,
			remotePeerAddress: peerAddress,
		}
		c.setPeerHostPort(span)
		span.Finish()
		rawSpan := tracer.FinishedSpans()[i]
		assert.Equal(t, test.wantHost, rawSpan.Tag(test.wantHostTag), "test %+v", test)
		if test.wantPort != 0 {
			assert.Equal(t, test.wantPort, rawSpan.Tag(string(ext.PeerPort)), "test %+v", test)
		} else {
			assert.Nil(t, rawSpan.Tag(string(ext.PeerPort)), "test %+v", test)
		}
	}
}

func TestExtractInboundSpanWithZipkinTracer(t *testing.T) {
	tracer := mocktracer.New()
	callReq := new(callReq)
	callReq.Tracing = Span{traceID: 1, spanID: 2, flags: 1}
	callReq.Headers = transportHeaders{
		ArgScheme:  string(JSON),
		CallerName: "caller",
	}
	peerInfo, peerAddress, err := parseRemotePeer(initParams{
		InitParamHostPort:    "host:123",
		InitParamProcessName: "test",
	}, &net.IPAddr{IP: net.ParseIP("1.1.1.1")})
	require.NoError(t, err, "Failed to parse remote peer info")
	c := Connection{
		channelConnectionCommon: channelConnectionCommon{
			log:    NullLogger,
			tracer: tracer,
		},
		remotePeerInfo:    peerInfo,
		remotePeerAddress: peerAddress,
	}

	// fail to extract with zipkin format, as MockTracer does not support it
	assert.Nil(t, c.extractInboundSpan(callReq), "zipkin format not available")

	// add zipkin format extractor and try again
	tracer.RegisterExtractor(zipkinSpanFormat, new(zipkinExtractor))
	span := c.extractInboundSpan(callReq)
	require.NotNil(t, span, "zipkin format available")

	// validate the extracted span was correctly populated
	s1, ok := span.(*mocktracer.MockSpan)
	require.True(t, ok)
	assert.Equal(t, 1, s1.SpanContext.TraceID)
	assert.Equal(t, 2, s1.ParentID)
	assert.True(t, s1.SpanContext.Sampled)
	assert.Equal(t, "", s1.OperationName, "operation name unknown initially")
	assert.Equal(t, string(JSON), s1.Tag("as"))
	assert.Equal(t, "caller", s1.Tag(string(ext.PeerService)))
	assert.Equal(t, "host", s1.Tag(string(ext.PeerHostname)))
	assert.Equal(t, uint16(123), s1.Tag(string(ext.PeerPort)))

	// start a temporary span so that we can populate headers with baggage
	tempSpan := tracer.StartSpan("test")
	tempSpan.SetBaggageItem("x", "y")
	headers := make(map[string]string)
	carrier := tracingHeadersCarrier(headers)
	err = tracer.Inject(tempSpan.Context(), opentracing.TextMap, carrier)
	assert.NoError(t, err)

	// run the public ExtractInboundSpan method with application headers
	inCall := &InboundCall{
		response: &InboundCallResponse{
			span: span,
		},
	}
	ctx := context.Background()
	ctx2 := ExtractInboundSpan(ctx, inCall, headers, tracer)
	span = opentracing.SpanFromContext(ctx2)
	s2, ok := span.(*mocktracer.MockSpan)
	require.True(t, ok)
	assert.Equal(t, s1, s2, "should be the same span started previously")
	assert.Equal(t, "y", s2.BaggageItem("x"), "baggage should've been added")
}

type zipkinInjector struct{}

func (z *zipkinInjector) Inject(sc mocktracer.MockSpanContext, carrier interface{}) error {
	span, ok := carrier.(*injectableSpan)
	if !ok {
		return opentracing.ErrInvalidCarrier
	}
	span.SetTraceID(uint64(sc.TraceID))
	span.SetSpanID(uint64(sc.SpanID))
	if sc.Sampled {
		span.SetFlags(1)
	} else {
		span.SetFlags(0)
	}
	return nil
}

type zipkinExtractor struct{}

func (z *zipkinExtractor) Extract(carrier interface{}) (mocktracer.MockSpanContext, error) {
	span, ok := carrier.(*Span)
	if !ok {
		return mocktracer.MockSpanContext{}, opentracing.ErrInvalidCarrier
	}
	return mocktracer.MockSpanContext{
		TraceID: int(span.traceID),
		SpanID:  int(span.spanID),
		Sampled: span.flags&1 == 1,
	}, nil
}
