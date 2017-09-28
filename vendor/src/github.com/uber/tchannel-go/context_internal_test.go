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
	"testing"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func TestNewContextBuilderDisableTracing(t *testing.T) {
	ctx, cancel := NewContextBuilder(time.Second).
		DisableTracing().Build()
	defer cancel()

	assert.True(t, isTracingDisabled(ctx), "Tracing should be disabled")
}

func TestCurrentSpan(t *testing.T) {
	ctx := context.Background()
	span := CurrentSpan(ctx)
	require.NotNil(t, span, "CurrentSpan() should always return something")

	tracer := mocktracer.New()
	sp := tracer.StartSpan("test")
	ctx = opentracing.ContextWithSpan(ctx, sp)
	span = CurrentSpan(ctx)
	require.NotNil(t, span, "CurrentSpan() should always return something")
	assert.EqualValues(t, 0, span.TraceID(), "mock tracer is not Zipkin-compatible")

	tracer.RegisterInjector(zipkinSpanFormat, new(zipkinInjector))
	span = CurrentSpan(ctx)
	require.NotNil(t, span, "CurrentSpan() should always return something")
	assert.NotEqual(t, uint64(0), span.TraceID(), "mock tracer is now Zipkin-compatible")
}

func TestContextWithoutHeadersKeyHeaders(t *testing.T) {
	ctx := WrapWithHeaders(context.Background(), map[string]string{"k1": "v1"})
	assert.Equal(t, map[string]string{"k1": "v1"}, ctx.Headers())
	ctx2 := WithoutHeaders(ctx)
	assert.Nil(t, ctx2.Value(contextKeyHeaders))
	_, ok := ctx2.(ContextWithHeaders)
	assert.False(t, ok)
}

func TestContextWithoutHeadersKeyTChannel(t *testing.T) {
	ctx, _ := NewContextBuilder(time.Second).SetShardKey("s1").Build()
	ctx2 := WithoutHeaders(ctx)
	assert.Nil(t, ctx2.Value(contextKeyTChannel))
	_, ok := ctx2.(ContextWithHeaders)
	assert.False(t, ok)
}
