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

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/crossdock/client"
	"github.com/uber/tchannel-go/crossdock/common"
	"github.com/uber/tchannel-go/crossdock/server"

	"github.com/crossdock/crossdock-go"
	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-client-go"
	"golang.org/x/net/context"
)

func TestTraceBehavior(t *testing.T) {
	tracer, tCloser := jaeger.NewTracer(
		"crossdock",
		jaeger.NewConstSampler(false),
		jaeger.NewNullReporter())
	defer tCloser.Close()

	s := &server.Server{
		HostPort: "127.0.0.1:0",
		Tracer:   tracer,
	}
	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	behavior := &Behavior{
		ServerPort:    s.Port(),
		Tracer:        tracer,
		ServiceToHost: func(server string) string { return "localhost" },
	}
	behavior.Register(s.Ch)

	c := &client.Client{
		ClientHostPort: "127.0.0.1:0",
		Behaviors: crossdock.Behaviors{
			BehaviorName: behavior.Run,
		},
	}
	err = c.Start()
	require.NoError(t, err)
	defer c.Close()

	crossdock.Wait(t, c.URL(), 10)

	behaviors := []struct {
		name string
		axes map[string][]string
	}{
		{
			name: BehaviorName,
			axes: map[string][]string{
				server1NameParam:     {common.DefaultServiceName},
				sampledParam:         {"true", "false"},
				server2NameParam:     {common.DefaultServiceName},
				server2EncodingParam: {string(tchannel.JSON), string(tchannel.Thrift)},
				server3NameParam:     {common.DefaultServiceName},
				server3EncodingParam: {string(tchannel.JSON), string(tchannel.Thrift)},
			},
		},
	}

	for _, bb := range behaviors {
		for _, entry := range crossdock.Combinations(bb.axes) {
			entryArgs := url.Values{}
			for k, v := range entry {
				entryArgs.Set(k, v)
			}
			// test via real HTTP call
			crossdock.Call(t, c.URL(), bb.name, entryArgs)
		}
	}
}

func TestNoSpanObserved(t *testing.T) {
	_, err := observeSpan(context.Background())
	assert.Equal(t, errNoSpanObserved, err)
}

func TestPrepareResponseErrors(t *testing.T) {
	b := &Behavior{}
	ctx := context.Background()
	_, err := b.prepareResponse(ctx, nil, nil)
	assert.Equal(t, errNoSpanObserved, err)

	span := opentracing.GlobalTracer().StartSpan("test")
	ctx = opentracing.ContextWithSpan(ctx, span)
	res, err := b.prepareResponse(ctx, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, "", res.Span.TraceID)

	_, err = b.prepareResponse(ctx, nil, &Downstream{
		Encoding: "invalid",
	})
	assert.Equal(t, errUnsupportedEncoding, err)
}

func TestTraceBehaviorRunErrors(t *testing.T) {
	b := &Behavior{}
	entries := crossdock.Run(crossdock.Params{
		sampledParam: "not a boolean",
	}, b.Run)
	assertFailedEntry(t, "Malformed param sampled", entries)

	params := crossdock.Params{
		server1NameParam:     common.DefaultServiceName,
		sampledParam:         "true",
		server2NameParam:     common.DefaultServiceName,
		server2EncodingParam: string(tchannel.JSON),
		server3NameParam:     common.DefaultServiceName,
		server3EncodingParam: string(tchannel.JSON),
	}

	b.Tracer = opentracing.GlobalTracer()
	b.jsonCall = func(ctx context.Context, downstream *Downstream) (*Response, error) {
		return nil, fmt.Errorf("made-up bad response")
	}
	entries = crossdock.Run(params, b.Run)
	assertFailedEntry(t, "Failed to startTrace", entries)

	b.jsonCall = func(ctx context.Context, downstream *Downstream) (*Response, error) {
		return &Response{
			Span: &ObservedSpan{},
		}, nil
	}
	entries = crossdock.Run(params, b.Run)
	assertFailedEntry(t, "Trace ID should not be empty", entries)
}

func assertFailedEntry(t *testing.T, expected string, entries []crossdock.Entry) {
	require.True(t, len(entries) > 0, "Must have some entries %+v", entries)
	entry := entries[len(entries)-1]
	require.EqualValues(t, "failed", entry["status"], "Entries: %v", entries)
	require.NotEmpty(t, entry["output"], "Entries: %v", entries)
	output := entry["output"].(string)
	assert.True(t, strings.Contains(output, expected), "Output must contain %s: %s", expected, output)
}
