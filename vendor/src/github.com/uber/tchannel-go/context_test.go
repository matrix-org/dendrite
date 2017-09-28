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
	"testing"
	"time"

	. "github.com/uber/tchannel-go"

	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"
	"github.com/uber/tchannel-go/testutils/goroutines"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

var cn = "hello"

func TestWrapContextForTest(t *testing.T) {
	call := testutils.NewIncomingCall(cn)
	ctx, cancel := NewContext(time.Second)
	defer cancel()
	actual := WrapContextForTest(ctx, call)
	assert.Equal(t, call, CurrentCall(actual), "Incorrect call object returned.")
}

func TestNewContextTimeoutZero(t *testing.T) {
	ctx, cancel := NewContextBuilder(0).Build()
	defer cancel()

	deadline, ok := ctx.Deadline()
	assert.True(t, ok, "Context missing deadline")
	assert.True(t, deadline.Sub(time.Now()) <= 0, "Deadline should be Now or earlier")
}

func TestRoutingDelegatePropagates(t *testing.T) {
	WithVerifiedServer(t, nil, func(ch *Channel, hostPort string) {
		peerInfo := ch.PeerInfo()
		testutils.RegisterFunc(ch, "test", func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
			return &raw.Res{
				Arg3: []byte(CurrentCall(ctx).RoutingDelegate()),
			}, nil
		})

		ctx, cancel := NewContextBuilder(time.Second).Build()
		defer cancel()
		_, arg3, _, err := raw.Call(ctx, ch, peerInfo.HostPort, peerInfo.ServiceName, "test", nil, nil)
		assert.NoError(t, err, "Call failed")
		assert.Equal(t, "", string(arg3), "Expected no routing delegate header")

		ctx, cancel = NewContextBuilder(time.Second).SetRoutingDelegate("xpr").Build()
		defer cancel()
		_, arg3, _, err = raw.Call(ctx, ch, peerInfo.HostPort, peerInfo.ServiceName, "test", nil, nil)
		assert.NoError(t, err, "Call failed")
		assert.Equal(t, "xpr", string(arg3), "Expected routing delegate header to be set")
	})
}

func TestRoutingKeyPropagates(t *testing.T) {
	WithVerifiedServer(t, nil, func(ch *Channel, hostPort string) {
		peerInfo := ch.PeerInfo()
		testutils.RegisterFunc(ch, "test", func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
			return &raw.Res{
				Arg3: []byte(CurrentCall(ctx).RoutingKey()),
			}, nil
		})

		ctx, cancel := NewContextBuilder(time.Second).Build()
		defer cancel()
		_, arg3, _, err := raw.Call(ctx, ch, peerInfo.HostPort, peerInfo.ServiceName, "test", nil, nil)
		assert.NoError(t, err, "Call failed")
		assert.Equal(t, "", string(arg3), "Expected no routing key header")

		ctx, cancel = NewContextBuilder(time.Second).SetRoutingKey("canary").Build()
		defer cancel()
		_, arg3, _, err = raw.Call(ctx, ch, peerInfo.HostPort, peerInfo.ServiceName, "test", nil, nil)
		assert.NoError(t, err, "Call failed")
		assert.Equal(t, "canary", string(arg3), "Expected routing key header to be set")
	})
}

func TestShardKeyPropagates(t *testing.T) {
	WithVerifiedServer(t, nil, func(ch *Channel, hostPort string) {
		peerInfo := ch.PeerInfo()
		testutils.RegisterFunc(ch, "test", func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
			return &raw.Res{
				Arg3: []byte(CurrentCall(ctx).ShardKey()),
			}, nil
		})

		ctx, cancel := NewContextBuilder(time.Second).Build()
		defer cancel()
		_, arg3, _, err := raw.Call(ctx, ch, peerInfo.HostPort, peerInfo.ServiceName, "test", nil, nil)
		assert.NoError(t, err, "Call failed")
		assert.Equal(t, arg3, []byte(""))

		ctx, cancel = NewContextBuilder(time.Second).
			SetShardKey("shard").Build()
		defer cancel()
		_, arg3, _, err = raw.Call(ctx, ch, peerInfo.HostPort, peerInfo.ServiceName, "test", nil, nil)
		assert.NoError(t, err, "Call failed")
		assert.Equal(t, string(arg3), "shard")
	})
}

func TestCurrentCallWithNilResult(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()
	call := CurrentCall(ctx)
	assert.Nil(t, call, "Should return nil.")
}

func getParentContext(t *testing.T) ContextWithHeaders {
	ctx := context.WithValue(context.Background(), "some key", "some value")
	assert.Equal(t, "some value", ctx.Value("some key"))

	ctx1, _ := NewContextBuilder(time.Second).
		SetParentContext(ctx).
		AddHeader("header key", "header value").
		Build()
	assert.Equal(t, "some value", ctx1.Value("some key"))
	return ctx1
}

func TestContextBuilderParentContextNoHeaders(t *testing.T) {
	ctx := getParentContext(t)
	assert.Equal(t, map[string]string{"header key": "header value"}, ctx.Headers())
	assert.EqualValues(t, "some value", ctx.Value("some key"), "inherited from parent ctx")
}

func TestContextBuilderParentContextMergeHeaders(t *testing.T) {
	ctx := getParentContext(t)
	ctx.Headers()["fixed header"] = "fixed value"

	// append header to parent
	ctx2, _ := NewContextBuilder(time.Second).
		SetParentContext(ctx).
		AddHeader("header key 2", "header value 2").
		Build()
	assert.Equal(t, map[string]string{
		"header key":   "header value",   // inherited
		"fixed header": "fixed value",    // inherited
		"header key 2": "header value 2", // appended
	}, ctx2.Headers())

	// override parent header
	ctx3, _ := NewContextBuilder(time.Second).
		SetParentContext(ctx).
		AddHeader("header key", "header value 2"). // override
		Build()

	assert.Equal(t, map[string]string{
		"header key":   "header value 2", // overwritten
		"fixed header": "fixed value",    // inherited
	}, ctx3.Headers())

	goroutines.VerifyNoLeaks(t, nil)
}

func TestContextBuilderParentContextReplaceHeaders(t *testing.T) {
	ctx := getParentContext(t)
	ctx.Headers()["fixed header"] = "fixed value"
	assert.Equal(t, map[string]string{
		"header key":   "header value",
		"fixed header": "fixed value",
	}, ctx.Headers())

	// replace headers with a new map
	ctx2, _ := NewContextBuilder(time.Second).
		SetParentContext(ctx).
		SetHeaders(map[string]string{"header key": "header value 2"}).
		Build()
	assert.Equal(t, map[string]string{"header key": "header value 2"}, ctx2.Headers())

	goroutines.VerifyNoLeaks(t, nil)
}

func TestContextWrapWithHeaders(t *testing.T) {
	headers1 := map[string]string{
		"k1": "v1",
	}
	ctx, _ := NewContextBuilder(time.Second).
		SetHeaders(headers1).
		Build()
	assert.Equal(t, headers1, ctx.Headers(), "Headers mismatch after Build")

	headers2 := map[string]string{
		"k1": "v1",
	}
	ctx2 := WrapWithHeaders(ctx, headers2)
	assert.Equal(t, headers2, ctx2.Headers(), "Headers mismatch after WrapWithHeaders")
}

func TestContextWithHeadersAsContext(t *testing.T) {
	var ctx context.Context = getParentContext(t)
	assert.EqualValues(t, "some value", ctx.Value("some key"), "inherited from parent ctx")
}

func TestContextBuilderParentContextSpan(t *testing.T) {
	ctx := getParentContext(t)
	assert.Equal(t, "some value", ctx.Value("some key"))

	ctx2, _ := NewContextBuilder(time.Second).
		SetParentContext(ctx).
		Build()
	assert.Equal(t, "some value", ctx2.Value("some key"), "key/value propagated from parent ctx")

	goroutines.VerifyNoLeaks(t, nil)
}

func TestContextWrapChild(t *testing.T) {
	tests := []struct {
		msg         string
		ctxFn       func() ContextWithHeaders
		wantHeaders map[string]string
		wantValue   interface{}
	}{
		{
			msg: "Basic context",
			ctxFn: func() ContextWithHeaders {
				ctxNoHeaders, _ := NewContextBuilder(time.Second).Build()
				return ctxNoHeaders
			},
			wantHeaders: nil,
			wantValue:   nil,
		},
		{
			msg: "Wrap basic context with value",
			ctxFn: func() ContextWithHeaders {
				ctxNoHeaders, _ := NewContextBuilder(time.Second).Build()
				return Wrap(context.WithValue(ctxNoHeaders, "1", "2"))
			},
			wantHeaders: nil,
			wantValue:   "2",
		},
		{
			msg: "Wrap context with headers and value",
			ctxFn: func() ContextWithHeaders {
				ctxWithHeaders, _ := NewContextBuilder(time.Second).AddHeader("h1", "v1").Build()
				return Wrap(context.WithValue(ctxWithHeaders, "1", "2"))
			},
			wantHeaders: map[string]string{"h1": "v1"},
			wantValue:   "2",
		},
	}

	for _, tt := range tests {
		for _, child := range []bool{false, true} {
			origCtx := tt.ctxFn()
			ctx := origCtx
			if child {
				ctx = origCtx.Child()
			}

			assert.Equal(t, tt.wantValue, ctx.Value("1"), "%v: Unexpected value", tt.msg)
			assert.Equal(t, tt.wantHeaders, ctx.Headers(), "%v: Unexpected headers", tt.msg)

			respHeaders := map[string]string{"r": "v"}
			ctx.SetResponseHeaders(respHeaders)
			assert.Equal(t, respHeaders, ctx.ResponseHeaders(), "%v: Unexpected response headers", tt.msg)

			if child {
				// If we're working with a child context, changes to response headers
				// should not affect the original context.
				assert.Nil(t, origCtx.ResponseHeaders(), "%v: Child modified original context's headers", tt.msg)
			}
		}
	}
}
