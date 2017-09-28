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

package thrift_test

import (
	"errors"
	"testing"
	"time"

	. "github.com/uber/tchannel-go/thrift"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"
	gen "github.com/uber/tchannel-go/thrift/gen-go/test"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestWrapContext(t *testing.T) {
	tctx, cancel := NewContext(time.Second)
	defer cancel()

	headers := map[string]string{"h1": "v1"}
	ctx := context.WithValue(WithHeaders(tctx, headers), "1", "2")
	wrapped := Wrap(ctx)
	assert.NotNil(t, wrapped, "Should not return nil.")

	assert.Equal(t, headers, wrapped.Headers(), "Unexpected headers")
	assert.Equal(t, "2", wrapped.Value("1"), "Unexpected value")
}

func TestContextBuilder(t *testing.T) {
	ctx, cancel := tchannel.NewContextBuilder(time.Second).SetShardKey("shard").Build()
	defer cancel()

	var called bool
	testutils.WithServer(t, nil, func(ch *tchannel.Channel, hostPort string) {
		peerInfo := ch.PeerInfo()

		testutils.RegisterFunc(ch, "SecondService::Echo", func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
			call := tchannel.CurrentCall(ctx)
			assert.Equal(t, peerInfo.ServiceName, call.CallerName(), "unexpected caller name")
			assert.Equal(t, "shard", call.ShardKey(), "unexpected shard key")
			assert.Equal(t, tchannel.Thrift, args.Format)
			called = true
			return nil, errors.New("err")
		})

		client := NewClient(ch, ch.PeerInfo().ServiceName, &ClientOptions{
			HostPort: peerInfo.HostPort,
		})
		secondClient := gen.NewTChanSecondServiceClient(client)
		secondClient.Echo(ctx, "asd")
		assert.True(t, called, "test not called")
	})
}
