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
	"strings"
	"testing"
	"time"

	. "github.com/uber/tchannel-go"

	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func TestActiveCallReq(t *testing.T) {
	t.Skip("Test skipped due to unreliable way to test for protocol errors")

	ctx, cancel := NewContext(time.Second)
	defer cancel()

	// Note: This test cannot use log verification as the duplicate ID causes a log.
	// It does not use a verified server, as it leaks a message exchange due to the
	// modification of IDs in the relay.
	opts := testutils.NewOpts().DisableLogVerification()
	testutils.WithServer(t, opts, func(ch *Channel, hostPort string) {
		gotCall := make(chan struct{})
		unblock := make(chan struct{})

		testutils.RegisterFunc(ch, "blocked", func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
			gotCall <- struct{}{}
			<-unblock
			return &raw.Res{}, nil
		})

		relayFunc := func(outgoing bool, frame *Frame) *Frame {
			if outgoing && frame.Header.ID == 3 {
				frame.Header.ID = 2
			}
			return frame
		}

		relayHostPort, closeRelay := testutils.FrameRelay(t, hostPort, relayFunc)
		defer closeRelay()

		firstComplete := make(chan struct{})
		go func() {
			// This call will block until we close unblock.
			raw.Call(ctx, ch, relayHostPort, ch.PeerInfo().ServiceName, "blocked", nil, nil)
			close(firstComplete)
		}()

		// Wait for the first call to be received by the server
		<-gotCall

		// Make a new call, which should fail
		_, _, _, err := raw.Call(ctx, ch, relayHostPort, ch.PeerInfo().ServiceName, "blocked", nil, nil)
		assert.Error(t, err, "Expect error")
		assert.True(t, strings.Contains(err.Error(), "already active"),
			"expected already active error, got %v", err)

		close(unblock)
		<-firstComplete
	})
}

func TestInboundConnection(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	// Disable relay since relays hide host:port on outbound calls.
	opts := testutils.NewOpts().NoRelay()
	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		s2 := ts.NewServer(nil)

		ts.RegisterFunc("test", func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
			c, _ := InboundConnection(CurrentCall(ctx))
			assert.Equal(t, s2.PeerInfo().HostPort, c.RemotePeerInfo().HostPort, "Unexpected host port")
			return &raw.Res{}, nil
		})

		_, _, _, err := raw.Call(ctx, s2, ts.HostPort(), ts.ServiceName(), "test", nil, nil)
		require.NoError(t, err, "Call failed")
	})
}

func TestInboundConnection_CallOptions(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	testutils.WithTestServer(t, nil, func(server *testutils.TestServer) {
		server.RegisterFunc("test", func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
			assert.Equal(t, "client", CurrentCall(ctx).CallerName(), "Expected caller name to be passed through")
			return &raw.Res{}, nil
		})

		backendName := server.ServiceName()

		proxyCh := server.NewServer(&testutils.ChannelOpts{ServiceName: "proxy"})
		defer proxyCh.Close()

		subCh := proxyCh.GetSubChannel(backendName)
		subCh.SetHandler(HandlerFunc(func(ctx context.Context, inbound *InboundCall) {
			outbound, err := proxyCh.BeginCall(ctx, server.HostPort(), backendName, inbound.MethodString(), inbound.CallOptions())
			require.NoError(t, err, "Create outbound call failed")
			arg2, arg3, _, err := raw.WriteArgs(outbound, []byte("hello"), []byte("world"))
			require.NoError(t, err, "Write outbound call failed")
			require.NoError(t, raw.WriteResponse(inbound.Response(), &raw.Res{
				Arg2: arg2,
				Arg3: arg3,
			}), "Write response failed")
		}))

		clientCh := server.NewClient(&testutils.ChannelOpts{
			ServiceName: "client",
		})
		defer clientCh.Close()

		_, _, _, err := raw.Call(ctx, clientCh, proxyCh.PeerInfo().HostPort, backendName, "test", nil, nil)
		require.NoError(t, err, "Call through proxy failed")
	})
}
