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
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/relay/relaytest"
	"github.com/uber/tchannel-go/testutils"
	"github.com/uber/tchannel-go/testutils/testreader"
	"github.com/uber/tchannel-go/tos"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// Values used in tests
var (
	testArg2 = []byte("Header in arg2")
	testArg3 = []byte("Body in arg3")
)

type testHandler struct {
	sync.Mutex

	t        testing.TB
	format   Format
	caller   string
	blockErr chan error
}

func newTestHandler(t testing.TB) *testHandler {
	return &testHandler{t: t, blockErr: make(chan error)}
}

func (h *testHandler) Handle(ctx context.Context, args *raw.Args) (*raw.Res, error) {
	h.Lock()
	h.format = args.Format
	h.caller = args.Caller
	h.Unlock()

	assert.Equal(h.t, args.Caller, CurrentCall(ctx).CallerName())

	switch args.Method {
	case "block":
		<-ctx.Done()
		h.blockErr <- ctx.Err()
		return &raw.Res{
			IsErr: true,
		}, nil
	case "echo":
		return &raw.Res{
			Arg2: args.Arg2,
			Arg3: args.Arg3,
		}, nil
	case "busy":
		return &raw.Res{
			SystemErr: ErrServerBusy,
		}, nil
	case "app-error":
		return &raw.Res{
			IsErr: true,
		}, nil
	}
	return nil, errors.New("unknown method")
}

func (h *testHandler) OnError(ctx context.Context, err error) {
	stack := make([]byte, 4096)
	runtime.Stack(stack, false /* all */)
	h.t.Errorf("testHandler got error: %v stack:\n%s", err, stack)
}

func writeFlushStr(w ArgWriter, d string) error {
	if _, err := io.WriteString(w, d); err != nil {
		return err
	}
	return w.Flush()
}

func isTosPriority(c net.Conn, tosPriority tos.ToS) (bool, error) {
	var connTosPriority int
	var err error

	switch ip := c.RemoteAddr().(*net.TCPAddr).IP; {
	case ip.To16() != nil && ip.To4() == nil:
		connTosPriority, err = ipv6.NewConn(c).TrafficClass()
	case ip.To4() != nil:
		connTosPriority, err = ipv4.NewConn(c).TOS()
	}

	return connTosPriority == int(tosPriority), err
}

func TestRoundTrip(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		handler := newTestHandler(t)
		ts.Register(raw.Wrap(handler), "echo")

		ctx, cancel := NewContext(time.Second)
		defer cancel()

		call, err := ts.Server().BeginCall(ctx, ts.HostPort(), ts.ServiceName(), "echo", &CallOptions{Format: JSON})
		require.NoError(t, err)
		assert.NotEmpty(t, call.RemotePeer().HostPort)
		assert.Equal(t, ts.Server().PeerInfo(), call.LocalPeer(), "Unexpected local peer")

		require.NoError(t, NewArgWriter(call.Arg2Writer()).Write(testArg2))
		require.NoError(t, NewArgWriter(call.Arg3Writer()).Write(testArg3))

		var respArg2 []byte
		require.NoError(t, NewArgReader(call.Response().Arg2Reader()).Read(&respArg2))
		assert.Equal(t, testArg2, []byte(respArg2))

		var respArg3 []byte
		require.NoError(t, NewArgReader(call.Response().Arg3Reader()).Read(&respArg3))
		assert.Equal(t, testArg3, []byte(respArg3))

		assert.Equal(t, JSON, handler.format)
		assert.Equal(t, ts.ServiceName(), handler.caller)
		assert.Equal(t, JSON, call.Response().Format(), "response Format should match request Format")
	})
}

func TestDefaultFormat(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		handler := newTestHandler(t)
		ts.Register(raw.Wrap(handler), "echo")

		ctx, cancel := NewContext(time.Second)
		defer cancel()

		arg2, arg3, resp, err := raw.Call(ctx, ts.Server(), ts.HostPort(), ts.ServiceName(), "echo", testArg2, testArg3)
		require.Nil(t, err)

		require.Equal(t, testArg2, arg2)
		require.Equal(t, testArg3, arg3)
		require.Equal(t, Raw, handler.format)
		assert.Equal(t, Raw, resp.Format(), "response Format should match request Format")
	})
}

func TestRemotePeer(t *testing.T) {
	wantVersion := PeerVersion{
		Language:        "go",
		LanguageVersion: strings.TrimPrefix(runtime.Version(), "go"),
		TChannelVersion: VersionInfo,
	}
	tests := []struct {
		name       string
		remote     func(*testutils.TestServer) *Channel
		expectedFn func(*RuntimeState, *testutils.TestServer) PeerInfo
	}{
		{
			name:   "ephemeral client",
			remote: func(ts *testutils.TestServer) *Channel { return ts.NewClient(nil) },
			expectedFn: func(state *RuntimeState, ts *testutils.TestServer) PeerInfo {
				return PeerInfo{
					HostPort:    state.RootPeers[ts.HostPort()].OutboundConnections[0].LocalHostPort,
					IsEphemeral: true,
					ProcessName: state.LocalPeer.ProcessName,
					Version:     wantVersion,
				}
			},
		},
		{
			name:   "listening server",
			remote: func(ts *testutils.TestServer) *Channel { return ts.NewServer(nil) },
			expectedFn: func(state *RuntimeState, ts *testutils.TestServer) PeerInfo {
				return PeerInfo{
					HostPort:    state.LocalPeer.HostPort,
					IsEphemeral: false,
					ProcessName: state.LocalPeer.ProcessName,
					Version:     wantVersion,
				}
			},
		},
	}

	ctx, cancel := NewContext(time.Second)
	defer cancel()

	for _, tt := range tests {
		opts := testutils.NewOpts().SetServiceName("fake-service").NoRelay()
		testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
			remote := tt.remote(ts)
			defer remote.Close()

			gotPeer := make(chan PeerInfo, 1)
			ts.RegisterFunc("test", func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
				gotPeer <- CurrentCall(ctx).RemotePeer()
				assert.Equal(t, ts.Server().PeerInfo(), CurrentCall(ctx).LocalPeer())
				return &raw.Res{}, nil
			})

			_, _, _, err := raw.Call(ctx, remote, ts.HostPort(), ts.Server().ServiceName(), "test", nil, nil)
			assert.NoError(t, err, "%v: Call failed", tt.name)
			expected := tt.expectedFn(remote.IntrospectState(nil), ts)
			assert.Equal(t, expected, <-gotPeer, "%v: RemotePeer mismatch", tt.name)
		})
	}
}

func TestReuseConnection(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	// Since we're specifically testing that connections between hosts are re-used,
	// we can't interpose a relay in this test.
	s1Opts := testutils.NewOpts().SetServiceName("s1").NoRelay()

	testutils.WithTestServer(t, s1Opts, func(ts *testutils.TestServer) {
		ch2 := ts.NewServer(&testutils.ChannelOpts{ServiceName: "s2"})
		hostPort2 := ch2.PeerInfo().HostPort
		defer ch2.Close()

		ts.Register(raw.Wrap(newTestHandler(t)), "echo")
		ch2.Register(raw.Wrap(newTestHandler(t)), "echo")

		outbound, err := ts.Server().BeginCall(ctx, hostPort2, "s2", "echo", nil)
		require.NoError(t, err)
		outboundConn, outboundNetConn := OutboundConnection(outbound)

		// Try to make another call at the same time, should reuse the same connection.
		outbound2, err := ts.Server().BeginCall(ctx, hostPort2, "s2", "echo", nil)
		require.NoError(t, err)
		outbound2Conn, _ := OutboundConnection(outbound)
		assert.Equal(t, outboundConn, outbound2Conn)

		// Wait for the connection to be marked as active in ch2.
		assert.True(t, testutils.WaitFor(time.Second, func() bool {
			return ch2.IntrospectState(nil).NumConnections > 0
		}), "ch2 does not have any active connections")

		// When ch2 tries to call the test server, it should reuse the existing
		// inbound connection the test server. Of course, this only works if the
		// test server -> ch2 call wasn't relayed.
		outbound3, err := ch2.BeginCall(ctx, ts.HostPort(), "s1", "echo", nil)
		require.NoError(t, err)
		_, outbound3NetConn := OutboundConnection(outbound3)
		assert.Equal(t, outboundNetConn.RemoteAddr(), outbound3NetConn.LocalAddr())
		assert.Equal(t, outboundNetConn.LocalAddr(), outbound3NetConn.RemoteAddr())

		// Ensure all calls can complete in parallel.
		var wg sync.WaitGroup
		for _, call := range []*OutboundCall{outbound, outbound2, outbound3} {
			wg.Add(1)
			go func(call *OutboundCall) {
				defer wg.Done()
				resp1, resp2, _, err := raw.WriteArgs(call, []byte("arg2"), []byte("arg3"))
				require.NoError(t, err)
				assert.Equal(t, resp1, []byte("arg2"), "result does match argument")
				assert.Equal(t, resp2, []byte("arg3"), "result does match argument")
			}(call)
		}
		wg.Wait()
	})
}

func TestPing(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		ctx, cancel := NewContext(time.Second)
		defer cancel()

		clientCh := ts.NewClient(nil)
		defer clientCh.Close()
		require.NoError(t, clientCh.Ping(ctx, ts.HostPort()))
	})
}

func TestBadRequest(t *testing.T) {
	// ch will log an error when it receives a request for an unknown handler.
	opts := testutils.NewOpts().AddLogFilter("Couldn't find handler.", 1)
	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		ctx, cancel := NewContext(time.Second)
		defer cancel()

		_, _, _, err := raw.Call(ctx, ts.Server(), ts.HostPort(), ts.ServiceName(), "Noone", []byte("Headers"), []byte("Body"))
		require.NotNil(t, err)
		assert.Equal(t, ErrCodeBadRequest, GetSystemErrorCode(err))

		calls := relaytest.NewMockStats()
		calls.Add(ts.ServiceName(), ts.ServiceName(), "Noone").Failed("bad-request").End()
		ts.AssertRelayStats(calls)
	})
}

func TestNoTimeout(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		ts.Register(raw.Wrap(newTestHandler(t)), "Echo")

		ctx := context.Background()
		_, _, _, err := raw.Call(ctx, ts.Server(), ts.HostPort(), "svc", "Echo", []byte("Headers"), []byte("Body"))
		assert.Equal(t, ErrTimeoutRequired, err)

		ts.AssertRelayStats(relaytest.NewMockStats())
	})
}

func TestCancelled(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		ts.Register(raw.Wrap(newTestHandler(t)), "echo")
		ctx, cancel := NewContext(time.Second)

		// Make a call first to make sure we have a connection.
		// We want to test the BeginCall path.
		_, _, _, err := raw.Call(ctx, ts.Server(), ts.HostPort(), ts.ServiceName(), "echo", []byte("Headers"), []byte("Body"))
		assert.NoError(t, err, "Call failed")

		// Now cancel the context.
		cancel()
		_, _, _, err = raw.Call(ctx, ts.Server(), ts.HostPort(), ts.ServiceName(), "echo", []byte("Headers"), []byte("Body"))
		assert.Equal(t, ErrRequestCancelled, err, "Unexpected error when making call with canceled context")
	})
}

func TestNoServiceNaming(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		ctx, cancel := NewContext(time.Second)
		defer cancel()

		_, _, _, err := raw.Call(ctx, ts.Server(), ts.HostPort(), "", "Echo", []byte("Headers"), []byte("Body"))
		assert.Equal(t, ErrNoServiceName, err)

		ts.AssertRelayStats(relaytest.NewMockStats())
	})
}

func TestServerBusy(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		ts.Register(ErrorHandlerFunc(func(ctx context.Context, call *InboundCall) error {
			if _, err := raw.ReadArgs(call); err != nil {
				return err
			}
			return ErrServerBusy
		}), "busy")

		ctx, cancel := NewContext(time.Second)
		defer cancel()

		_, _, _, err := raw.Call(ctx, ts.Server(), ts.HostPort(), ts.ServiceName(), "busy", []byte("Arg2"), []byte("Arg3"))
		require.NotNil(t, err)
		assert.Equal(t, ErrCodeBusy, GetSystemErrorCode(err), "err: %v", err)

		calls := relaytest.NewMockStats()
		calls.Add(ts.ServiceName(), ts.ServiceName(), "busy").Failed("busy").End()
		ts.AssertRelayStats(calls)
	})
}

func TestUnexpectedHandlerError(t *testing.T) {
	opts := testutils.NewOpts().
		AddLogFilter("Unexpected handler error", 1)

	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		ts.Register(ErrorHandlerFunc(func(ctx context.Context, call *InboundCall) error {
			if _, err := raw.ReadArgs(call); err != nil {
				return err
			}
			return fmt.Errorf("nope")
		}), "nope")

		ctx, cancel := NewContext(time.Second)
		defer cancel()

		_, _, _, err := raw.Call(ctx, ts.Server(), ts.HostPort(), ts.ServiceName(), "nope", []byte("Arg2"), []byte("Arg3"))
		require.NotNil(t, err)
		assert.Equal(t, ErrCodeUnexpected, GetSystemErrorCode(err), "err: %v", err)

		calls := relaytest.NewMockStats()
		calls.Add(ts.ServiceName(), ts.ServiceName(), "nope").Failed("unexpected-error").End()
		ts.AssertRelayStats(calls)
	})
}

type onErrorTestHandler struct {
	*testHandler
	onError func(ctx context.Context, err error)
}

func (h onErrorTestHandler) OnError(ctx context.Context, err error) {
	h.onError(ctx, err)
}

func TestTimeout(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		// onError may be called when the block call tries to write the call response.
		onError := func(ctx context.Context, err error) {
			assert.Equal(t, ErrTimeout, err, "onError err should be ErrTimeout")
			assert.Equal(t, context.DeadlineExceeded, ctx.Err(), "Context should timeout")
		}
		testHandler := onErrorTestHandler{newTestHandler(t), onError}
		ts.Register(raw.Wrap(testHandler), "block")

		ctx, cancel := NewContext(testutils.Timeout(15 * time.Millisecond))
		defer cancel()

		_, _, _, err := raw.Call(ctx, ts.Server(), ts.HostPort(), ts.ServiceName(), "block", []byte("Arg2"), []byte("Arg3"))
		assert.Equal(t, ErrTimeout, err)

		// Verify the server-side receives an error from the context.
		select {
		case err := <-testHandler.blockErr:
			assert.Equal(t, context.DeadlineExceeded, err, "Server should have received timeout")
		case <-time.After(time.Second):
			t.Errorf("Server did not receive call, may need higher timeout")
		}

		calls := relaytest.NewMockStats()
		calls.Add(ts.ServiceName(), ts.ServiceName(), "block").Failed("timeout").End()
		ts.AssertRelayStats(calls)
	})
}

func TestLargeMethod(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		ctx, cancel := NewContext(time.Second)
		defer cancel()

		largeMethod := testutils.RandBytes(16*1024 + 1)
		_, _, _, err := raw.Call(ctx, ts.Server(), ts.HostPort(), ts.ServiceName(), string(largeMethod), nil, nil)
		assert.Equal(t, ErrMethodTooLarge, err)
	})
}

func TestLargeTimeout(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		ts.Register(raw.Wrap(newTestHandler(t)), "echo")

		ctx, cancel := NewContext(1000 * time.Second)
		defer cancel()

		_, _, _, err := raw.Call(ctx, ts.Server(), ts.HostPort(), ts.ServiceName(), "echo", testArg2, testArg3)
		assert.NoError(t, err, "Call failed")

		calls := relaytest.NewMockStats()
		calls.Add(ts.ServiceName(), ts.ServiceName(), "echo").Succeeded().End()
		ts.AssertRelayStats(calls)
	})
}

func TestFragmentation(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		ts.Register(raw.Wrap(newTestHandler(t)), "echo")

		arg2 := make([]byte, MaxFramePayloadSize*2)
		for i := 0; i < len(arg2); i++ {
			arg2[i] = byte('a' + (i % 10))
		}

		arg3 := make([]byte, MaxFramePayloadSize*3)
		for i := 0; i < len(arg3); i++ {
			arg3[i] = byte('A' + (i % 10))
		}

		ctx, cancel := NewContext(time.Second)
		defer cancel()

		respArg2, respArg3, _, err := raw.Call(ctx, ts.Server(), ts.HostPort(), ts.ServiceName(), "echo", arg2, arg3)
		require.NoError(t, err)
		assert.Equal(t, arg2, respArg2)
		assert.Equal(t, arg3, respArg3)

		calls := relaytest.NewMockStats()
		calls.Add(ts.ServiceName(), ts.ServiceName(), "echo").Succeeded().End()
		ts.AssertRelayStats(calls)
	})
}

func TestFragmentationSlowReader(t *testing.T) {
	// Inbound forward will timeout and cause a warning log.
	opts := testutils.NewOpts().
		AddLogFilter("Unable to forward frame", 1).
		AddLogFilter("Connection error", 1)

	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		startReading, handlerComplete := make(chan struct{}), make(chan struct{})
		handler := func(ctx context.Context, call *InboundCall) {
			<-startReading
			<-ctx.Done()
			_, err := raw.ReadArgs(call)
			assert.Error(t, err, "ReadArgs should fail since frames will be dropped due to slow reading")
			close(handlerComplete)
		}

		ts.Register(HandlerFunc(handler), "echo")

		arg2 := testutils.RandBytes(MaxFramePayloadSize * MexChannelBufferSize)
		arg3 := testutils.RandBytes(MaxFramePayloadSize * (MexChannelBufferSize + 1))

		ctx, cancel := NewContext(testutils.Timeout(30 * time.Millisecond))
		defer cancel()

		_, _, _, err := raw.Call(ctx, ts.Server(), ts.HostPort(), ts.ServiceName(), "echo", arg2, arg3)
		assert.Error(t, err, "Call should timeout due to slow reader")

		close(startReading)
		select {
		case <-handlerComplete:
		case <-time.After(testutils.Timeout(70 * time.Millisecond)):
			t.Errorf("Handler not called, context timeout may be too low")
		}

		calls := relaytest.NewMockStats()
		calls.Add(ts.ServiceName(), ts.ServiceName(), "echo").Failed("timeout").End()
		ts.AssertRelayStats(calls)
	})
}

func TestWriteArg3AfterTimeout(t *testing.T) {
	// The channel reads and writes during timeouts, causing warning logs.
	opts := testutils.NewOpts().DisableLogVerification()
	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		timedOut := make(chan struct{})

		handler := func(ctx context.Context, call *InboundCall) {
			_, err := raw.ReadArgs(call)
			assert.NoError(t, err, "Read args failed")
			response := call.Response()
			assert.NoError(t, NewArgWriter(response.Arg2Writer()).Write(nil), "Write Arg2 failed")
			writer, err := response.Arg3Writer()
			assert.NoError(t, err, "Arg3Writer failed")

			for {
				if _, err := writer.Write(testutils.RandBytes(4)); err != nil {
					assert.Equal(t, err, ErrTimeout, "Handler should timeout")
					close(timedOut)
					return
				}
				runtime.Gosched()
			}
		}
		ts.Register(HandlerFunc(handler), "call")

		ctx, cancel := NewContext(testutils.Timeout(100 * time.Millisecond))
		defer cancel()

		_, _, _, err := raw.Call(ctx, ts.Server(), ts.HostPort(), ts.ServiceName(), "call", nil, nil)
		assert.Equal(t, err, ErrTimeout, "Call should timeout")

		// Wait for the write to complete, make sure there are no errors.
		select {
		case <-time.After(testutils.Timeout(60 * time.Millisecond)):
			t.Errorf("Handler should have failed due to timeout")
		case <-timedOut:
		}

		calls := relaytest.NewMockStats()
		calls.Add(ts.ServiceName(), ts.ServiceName(), "call").Failed("timeout").Succeeded().End()
		ts.AssertRelayStats(calls)
	})
}

func TestWriteErrorAfterTimeout(t *testing.T) {
	// TODO: Make this test block at different points (e.g. before, during read/write).
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		timedOut := make(chan struct{})
		done := make(chan struct{})
		handler := func(ctx context.Context, call *InboundCall) {
			<-ctx.Done()
			<-timedOut
			_, err := raw.ReadArgs(call)
			assert.Equal(t, ErrTimeout, err, "Read args should fail with timeout")
			response := call.Response()
			assert.Equal(t, ErrTimeout, response.SendSystemError(ErrServerBusy), "SendSystemError should fail")
			close(done)
		}
		ts.Register(HandlerFunc(handler), "call")

		ctx, cancel := NewContext(testutils.Timeout(30 * time.Millisecond))
		defer cancel()
		_, _, _, err := raw.Call(ctx, ts.Server(), ts.HostPort(), ts.ServiceName(), "call", nil, testutils.RandBytes(100000))
		assert.Equal(t, err, ErrTimeout, "Call should timeout")
		close(timedOut)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Errorf("Handler not called, timeout may be too low")
		}

		calls := relaytest.NewMockStats()
		calls.Add(ts.ServiceName(), ts.ServiceName(), "call").Failed("timeout").End()
		ts.AssertRelayStats(calls)
	})
}

func TestWriteAfterConnectionError(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	// Closing network connections can lead to warnings in many places.
	// TODO: Relay is disabled due to https://github.com/uber/tchannel-go/issues/390
	// Enabling relay causes the test to be flaky.
	opts := testutils.NewOpts().DisableLogVerification().NoRelay()
	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		testutils.RegisterEcho(ts.Server(), nil)
		server := ts.Server()

		call, err := server.BeginCall(ctx, ts.HostPort(), server.ServiceName(), "echo", nil)
		require.NoError(t, err, "Call failed")

		w, err := call.Arg2Writer()
		require.NoError(t, err, "Arg2Writer failed")
		require.NoError(t, writeFlushStr(w, "initial"), "write initial failed")

		// Now close the underlying network connection, writes should fail.
		_, conn := OutboundConnection(call)
		conn.Close()

		// Writes should start failing pretty soon.
		var writeErr error
		for i := 0; i < 100; i++ {
			if writeErr = writeFlushStr(w, "f"); writeErr != nil {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if assert.Error(t, writeErr, "Writes should fail after a connection is closed") {
			assert.Equal(t, ErrCodeNetwork, GetSystemErrorCode(writeErr), "write should fail due to network error")
		}
	})
}

func TestReadTimeout(t *testing.T) {
	// The error frame may fail to send since the connection closes before the handler sends it
	// or the handler connection may be closed as it sends when the other side closes the conn.
	opts := testutils.NewOpts().
		AddLogFilter("Couldn't send outbound error frame", 1).
		AddLogFilter("Connection error", 1, "site", "read frames").
		AddLogFilter("Connection error", 1, "site", "write frames").
		AddLogFilter("simpleHandler OnError", 1,
			"error", "failed to send error frame, connection state connectionClosed")

	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		sn := ts.ServiceName()
		calls := relaytest.NewMockStats()

		for i := 0; i < 10; i++ {
			ctx, cancel := NewContext(time.Second)
			handler := func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
				defer cancel()
				return nil, ErrRequestCancelled
			}
			ts.RegisterFunc("call", handler)

			_, _, _, err := raw.Call(ctx, ts.Server(), ts.HostPort(), ts.ServiceName(), "call", nil, nil)
			assert.Equal(t, err, ErrRequestCancelled, "Call should fail due to cancel")
			calls.Add(sn, sn, "call").Failed("cancelled").End()
		}

		ts.AssertRelayStats(calls)
	})
}

func TestWriteTimeout(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		ch := ts.Server()
		ctx, cancel := NewContext(testutils.Timeout(15 * time.Millisecond))
		defer cancel()

		call, err := ch.BeginCall(ctx, ts.HostPort(), ch.ServiceName(), "call", nil)
		require.NoError(t, err, "Call failed")

		writer, err := call.Arg2Writer()
		require.NoError(t, err, "Arg2Writer failed")

		_, err = writer.Write([]byte{1})
		require.NoError(t, err, "Write initial bytes failed")
		<-ctx.Done()

		_, err = io.Copy(writer, testreader.Looper([]byte{1}))
		assert.Equal(t, ErrTimeout, err, "Write should fail with timeout")

		ts.AssertRelayStats(relaytest.NewMockStats())
	})
}

func TestGracefulClose(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		ch2 := ts.NewServer(nil)
		hp2 := ch2.PeerInfo().HostPort
		defer ch2.Close()

		ctx, cancel := NewContext(time.Second)
		defer cancel()

		assert.NoError(t, ts.Server().Ping(ctx, hp2), "Ping from ch1 -> ch2 failed")
		assert.NoError(t, ch2.Ping(ctx, ts.HostPort()), "Ping from ch2 -> ch1 failed")

		// No stats for pings.
		ts.AssertRelayStats(relaytest.NewMockStats())
	})
}

func TestNetDialTimeout(t *testing.T) {
	// timeoutHostPort uses a blackholed address (RFC 6890) with a port
	// reserved for documentation. This address should always cause a timeout.
	const timeoutHostPort = "192.18.0.254:44444"
	timeoutPeriod := testutils.Timeout(50 * time.Millisecond)

	client := testutils.NewClient(t, nil)
	defer client.Close()

	started := time.Now()
	ctx, cancel := NewContext(timeoutPeriod)
	defer cancel()

	err := client.Ping(ctx, timeoutHostPort)
	if !assert.Error(t, err, "Ping to blackhole address should fail") {
		return
	}

	if strings.Contains(err.Error(), "network is unreachable") {
		t.Skipf("Skipping test, as network interface may not be available")
	}

	d := time.Since(started)
	assert.Equal(t, ErrTimeout, err, "Ping expected to fail with timeout")
	assert.True(t, d >= timeoutPeriod, "Timeout should take more than %v, took %v", timeoutPeriod, d)
}

func TestConnectTimeout(t *testing.T) {
	opts := testutils.NewOpts().DisableLogVerification()
	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		// Set up a relay that will delay the initial init req.
		testComplete := make(chan struct{})

		relayFunc := func(outgoing bool, f *Frame) *Frame {
			select {
			case <-time.After(testutils.Timeout(200 * time.Millisecond)):
				return f
			case <-testComplete:
				// TODO: We should be able to forward the frame and have this test not fail.
				// Currently, it fails since the sequence of events is:
				// Server receives a TCP connection
				// Channel.Close() is called on the server
				// Server's TCP connection receives an init req
				// Since we don't currently track pending connections, the open TCP connection is not closed, and
				// we process the init req. This leaves an open connection at the end of the test.
				return nil
			}
		}
		relay, shutdown := testutils.FrameRelay(t, ts.HostPort(), relayFunc)
		defer shutdown()

		// Make a call with a long timeout, but short connect timeout.
		// We expect the call to fall almost immediately with ErrTimeout.
		ctx, cancel := NewContextBuilder(2 * time.Second).
			SetConnectTimeout(testutils.Timeout(100 * time.Millisecond)).
			Build()
		defer cancel()

		client := ts.NewClient(opts)
		err := client.Ping(ctx, relay)
		assert.Equal(t, ErrTimeout, err, "Ping should timeout due to timeout relay")

		// Note: we do not defer this, as we need to close(testComplete) before
		// we call shutdown since shutdown waits for the relay to close, which
		// is stuck waiting inside of our custom relay function.
		close(testComplete)
	})
}

func TestParallelConnectionAccepts(t *testing.T) {
	opts := testutils.NewOpts().AddLogFilter("Failed during connection handshake", 1)
	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		testutils.RegisterEcho(ts.Server(), nil)

		// Start a connection attempt that should timeout.
		conn, err := net.Dial("tcp", ts.HostPort())
		defer conn.Close()
		require.NoError(t, err, "Dial failed")

		// When we try to make a call using a new client, it will require a
		// new connection, and this verifies that the previous connection attempt
		// and handshake do not impact the call.
		client := ts.NewClient(nil)
		testutils.AssertEcho(t, client, ts.HostPort(), ts.ServiceName())
	})
}

func TestConnectionIDs(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		var inbound, outbound []uint32
		relayFunc := func(outgoing bool, f *Frame) *Frame {
			if outgoing {
				outbound = append(outbound, f.Header.ID)
			} else {
				inbound = append(inbound, f.Header.ID)
			}
			return f
		}
		relay, shutdown := testutils.FrameRelay(t, ts.HostPort(), relayFunc)
		defer shutdown()

		ctx, cancel := NewContext(time.Second)
		defer cancel()

		s2 := ts.NewServer(nil)
		require.NoError(t, s2.Ping(ctx, relay), "Ping failed")
		assert.Equal(t, []uint32{1, 2}, outbound, "Unexpected outbound IDs")
		assert.Equal(t, []uint32{1, 2}, inbound, "Unexpected outbound IDs")

		// We want to reuse the same connection for the rest of the test which
		// only makes sense when the relay is not used.
		if ts.Relay() != nil {
			return
		}

		inbound = nil
		outbound = nil
		// We will reuse the inbound connection, but since the inbound connection
		// hasn't originated any outbound requests, we'll use id 1.
		require.NoError(t, ts.Server().Ping(ctx, s2.PeerInfo().HostPort), "Ping failed")
		assert.Equal(t, []uint32{1}, outbound, "Unexpected outbound IDs")
		assert.Equal(t, []uint32{1}, inbound, "Unexpected outbound IDs")
	})
}

func TestTosPriority(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	opts := testutils.NewOpts().SetServiceName("s1").SetTosPriority(tos.Lowdelay)
	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		ts.Register(raw.Wrap(newTestHandler(t)), "echo")

		outbound, err := ts.Server().BeginCall(ctx, ts.HostPort(), "s1", "echo", nil)
		require.NoError(t, err, "BeginCall failed")

		_, outboundNetConn := OutboundConnection(outbound)
		connTosPriority, err := isTosPriority(outboundNetConn, tos.Lowdelay)
		require.NoError(t, err, "Checking TOS priority failed")
		assert.Equal(t, connTosPriority, true)
		_, _, _, err = raw.WriteArgs(outbound, []byte("arg2"), []byte("arg3"))
		require.NoError(t, err, "Failed to write to outbound conn")
	})
}

func TestPeerStatusChangeClientReduction(t *testing.T) {
	sopts := testutils.NewOpts().NoRelay()
	testutils.WithTestServer(t, sopts, func(ts *testutils.TestServer) {
		server := ts.Server()
		testutils.RegisterEcho(server, nil)
		changes := make(chan int, 2)

		copts := testutils.NewOpts().SetOnPeerStatusChanged(func(p *Peer) {
			i, o := p.NumConnections()
			assert.Equal(t, 0, i, "no inbound connections to client")
			changes <- o
		})

		// Induce the creation of a connection from client to server.
		client := ts.NewClient(copts)
		require.NoError(t, testutils.CallEcho(client, ts.HostPort(), ts.ServiceName(), nil))
		assert.Equal(t, 1, <-changes, "event for first connection")

		// Re-use
		testutils.AssertEcho(t, client, ts.HostPort(), ts.ServiceName())

		// Induce the destruction of a connection from the server to the client.
		server.Close()
		assert.Equal(t, 0, <-changes, "event for second disconnection")

		client.Close()
		assert.Len(t, changes, 0, "unexpected peer status changes")
	})
}

func TestPeerStatusChangeClient(t *testing.T) {
	sopts := testutils.NewOpts().NoRelay()
	testutils.WithTestServer(t, sopts, func(ts *testutils.TestServer) {
		server := ts.Server()
		testutils.RegisterEcho(server, nil)
		changes := make(chan int, 2)

		copts := testutils.NewOpts().SetOnPeerStatusChanged(func(p *Peer) {
			i, o := p.NumConnections()
			assert.Equal(t, 0, i, "no inbound connections to client")
			changes <- o
		})

		// Induce the creation of a connection from client to server.
		client := ts.NewClient(copts)
		require.NoError(t, testutils.CallEcho(client, ts.HostPort(), ts.ServiceName(), nil))
		assert.Equal(t, 1, <-changes, "event for first connection")

		// Re-use
		testutils.AssertEcho(t, client, ts.HostPort(), ts.ServiceName())

		// Induce the creation of a second connection from client to server.
		pl := client.RootPeers()
		p := pl.GetOrAdd(ts.HostPort())
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, testutils.Timeout(100*time.Millisecond))
		defer cancel()
		_, err := p.Connect(ctx)
		require.NoError(t, err)
		assert.Equal(t, 2, <-changes, "event for second connection")

		// Induce the destruction of a connection from the server to the client.
		server.Close()
		<-changes // May be 1 or 0 depending on timing.
		assert.Equal(t, 0, <-changes, "event for second disconnection")

		client.Close()
		assert.Len(t, changes, 0, "unexpected peer status changes")
	})
}

func TestPeerStatusChangeServer(t *testing.T) {
	changes := make(chan int, 10)
	sopts := testutils.NewOpts().NoRelay().SetOnPeerStatusChanged(func(p *Peer) {
		i, o := p.NumConnections()
		assert.Equal(t, 0, o, "no outbound connections from server")
		changes <- i
	})
	testutils.WithTestServer(t, sopts, func(ts *testutils.TestServer) {
		server := ts.Server()
		testutils.RegisterEcho(server, nil)

		copts := testutils.NewOpts()
		for i := 0; i < 5; i++ {
			client := ts.NewClient(copts)

			// Open
			testutils.AssertEcho(t, client, ts.HostPort(), ts.ServiceName())
			assert.Equal(t, 1, <-changes, "one event on new connection")

			// Re-use
			testutils.AssertEcho(t, client, ts.HostPort(), ts.ServiceName())
			assert.Len(t, changes, 0, "no new events on re-used connection")

			// Close
			client.Close()
			assert.Equal(t, 0, <-changes, "one event on lost connection")
		}
	})
	assert.Len(t, changes, 0, "unexpected peer status changes")
}

func TestContextCanceledOnTCPClose(t *testing.T) {
	// 1. Context canceled warning is expected as part of this test
	// add log filter to ignore this error
	// 2. We use our own relay in this test, so disable the relay
	// that comes with the test server
	opts := testutils.NewOpts().NoRelay().AddLogFilter("simpleHandler OnError", 1)

	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		serverDoneC := make(chan struct{})
		callForwarded := make(chan struct{})

		ts.RegisterFunc("test", func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
			defer close(serverDoneC)
			close(callForwarded)
			<-ctx.Done()
			assert.EqualError(t, ctx.Err(), "context canceled")
			return &raw.Res{}, nil
		})

		// Set up a relay that can be used to terminate conns
		// on both sides i.e. client and server
		relayFunc := func(outgoing bool, f *Frame) *Frame {
			return f
		}
		relayHostPort, shutdown := testutils.FrameRelay(t, ts.HostPort(), relayFunc)

		// Make a call with a long timeout. We shutdown the relay
		// immediately after the server receives the call. Expected
		// behavior is for both client/server to be done with the call
		// immediately after relay shutsdown
		ctx, cancel := NewContext(20 * time.Second)
		defer cancel()

		clientCh := ts.NewClient(nil)
		// initiate the call in a background routine and
		// make it wait for the response
		clientDoneC := make(chan struct{})
		go func() {
			raw.Call(ctx, clientCh, relayHostPort, ts.ServiceName(), "test", nil, nil)
			close(clientDoneC)
		}()

		// wait for server to receive the call
		select {
		case <-callForwarded:
		case <-time.After(2 * time.Second):
			assert.Fail(t, "timed waiting for call to be forwarded")
		}

		// now shutdown the relay to close conns
		// on both sides
		shutdown()

		// wait for both the client & server to be done
		select {
		case <-serverDoneC:
		case <-time.After(2 * time.Second):
			assert.Fail(t, "timed out waiting for server handler to exit")
		}

		select {
		case <-clientDoneC:
		case <-time.After(2 * time.Second):
			assert.Fail(t, "timed out waiting for client to exit")
		}

		clientCh.Close()
	})
}
