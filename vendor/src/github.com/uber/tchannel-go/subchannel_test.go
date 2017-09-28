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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

type chanSet struct {
	main     Registrar
	sub      Registrar
	isolated Registrar
}

func withNewSet(t *testing.T, f func(*testing.T, chanSet)) {
	ch := testutils.NewClient(t, nil)
	defer ch.Close()
	f(t, chanSet{
		main:     ch,
		sub:      ch.GetSubChannel("hyperbahn"),
		isolated: ch.GetSubChannel("ringpop", Isolated),
	})
}

// Assert that two Registrars have references to the same Peer.
func assertHaveSameRef(t *testing.T, r1, r2 Registrar) {
	p1, err := r1.Peers().Get(nil)
	assert.NoError(t, err, "First registrar has no peers.")

	p2, err := r2.Peers().Get(nil)
	assert.NoError(t, err, "Second registrar has no peers.")

	assert.True(t, p1 == p2, "Registrars have references to different peers.")
}

func assertNoPeer(t *testing.T, r Registrar) {
	_, err := r.Peers().Get(nil)
	assert.Equal(t, err, ErrNoPeers)
}

func TestMainAddVisibility(t *testing.T) {
	withNewSet(t, func(t *testing.T, set chanSet) {
		// Adding a peer to the main channel should be reflected in the
		// subchannel, but not the isolated subchannel.
		set.main.Peers().Add("127.0.0.1:3000")
		assertHaveSameRef(t, set.main, set.sub)
		assertNoPeer(t, set.isolated)
	})
}

func TestSubchannelAddVisibility(t *testing.T) {
	withNewSet(t, func(t *testing.T, set chanSet) {
		// Adding a peer to a non-isolated subchannel should be reflected in
		// the main channel but not in isolated siblings.
		set.sub.Peers().Add("127.0.0.1:3000")
		assertHaveSameRef(t, set.main, set.sub)
		assertNoPeer(t, set.isolated)
	})
}

func TestIsolatedAddVisibility(t *testing.T) {
	withNewSet(t, func(t *testing.T, set chanSet) {
		// Adding a peer to an isolated subchannel shouldn't change the main
		// channel or sibling channels.
		set.isolated.Peers().Add("127.0.0.1:3000")

		_, err := set.isolated.Peers().Get(nil)
		assert.NoError(t, err)

		assertNoPeer(t, set.main)
		assertNoPeer(t, set.sub)
	})
}

func TestAddReusesPeers(t *testing.T) {
	withNewSet(t, func(t *testing.T, set chanSet) {
		// Adding to both a channel and an isolated subchannel shouldn't create
		// two separate peers.
		set.main.Peers().Add("127.0.0.1:3000")
		set.isolated.Peers().Add("127.0.0.1:3000")

		assertHaveSameRef(t, set.main, set.sub)
		assertHaveSameRef(t, set.main, set.isolated)
	})
}

func TestSetHandler(t *testing.T) {
	// Generate a Handler that expects only the given methods to be called.
	genHandler := func(methods ...string) Handler {
		allowedMethods := make(map[string]struct{}, len(methods))
		for _, m := range methods {
			allowedMethods[m] = struct{}{}
		}

		return HandlerFunc(func(ctx context.Context, call *InboundCall) {
			method := call.MethodString()
			assert.Contains(t, allowedMethods, method, "unexpected call to %q", method)
			err := raw.WriteResponse(call.Response(), &raw.Res{Arg3: []byte(method)})
			require.NoError(t, err)
		})
	}

	ch := testutils.NewServer(t, testutils.NewOpts().
		AddLogFilter("Couldn't find handler", 1, "serviceName", "svc2", "method", "bar"))
	defer ch.Close()

	// Catch-all handler for the main channel that accepts foo, bar, and baz,
	// and a single registered handler for a different subchannel.
	ch.GetSubChannel("svc1").SetHandler(genHandler("foo", "bar", "baz"))
	ch.GetSubChannel("svc2").Register(genHandler("foo"), "foo")

	client := testutils.NewClient(t, nil)
	client.Peers().Add(ch.PeerInfo().HostPort)
	defer client.Close()

	tests := []struct {
		Service    string
		Method     string
		ShouldFail bool
	}{
		{"svc1", "foo", false},
		{"svc1", "bar", false},
		{"svc1", "baz", false},

		{"svc2", "foo", false},
		{"svc2", "bar", true},
	}

	for _, tt := range tests {
		c := client.GetSubChannel(tt.Service)
		ctx, _ := NewContext(time.Second)
		_, data, _, err := raw.CallSC(ctx, c, tt.Method, nil, []byte("irrelevant"))

		if tt.ShouldFail {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			assert.Equal(t, tt.Method, string(data))
		}
	}

	st := ch.IntrospectState(nil)
	assert.Equal(t, "overriden", st.SubChannels["svc1"].Handler.Type.String())
	assert.Nil(t, st.SubChannels["svc1"].Handler.Methods)

	assert.Equal(t, "methods", st.SubChannels["svc2"].Handler.Type.String())
	assert.Equal(t, []string{"foo"}, st.SubChannels["svc2"].Handler.Methods)
}

func TestGetHandlers(t *testing.T) {
	ch := testutils.NewServer(t, nil)
	defer ch.Close()

	var handler1 HandlerFunc = func(_ context.Context, _ *InboundCall) {
		panic("unexpected call")
	}
	var handler2 HandlerFunc = func(_ context.Context, _ *InboundCall) {
		panic("unexpected call")
	}

	ch.Register(handler1, "method1")
	ch.Register(handler2, "method2")
	ch.GetSubChannel("foo").Register(handler2, "method1")

	tests := []struct {
		serviceName string
		wantMethods []string
	}{
		{
			serviceName: ch.ServiceName(),
			// Default service name comes with extra introspection methods.
			wantMethods: []string{"_gometa_introspect", "_gometa_runtime", "method1", "method2"},
		},
		{
			serviceName: "foo",
			wantMethods: []string{"method1"},
		},
	}

	for _, tt := range tests {
		handlers := ch.GetSubChannel(tt.serviceName).GetHandlers()
		if !assert.Equal(t, len(tt.wantMethods), len(handlers),
			"Unexpected number of methods found, expected %v, got %v", tt.wantMethods, handlers) {
			continue
		}

		for _, method := range tt.wantMethods {
			_, ok := handlers[method]
			assert.True(t, ok, "Expected to find method %v in handlers: %v", method, handlers)
		}
	}
}

func TestCannotRegisterOrGetAfterSetHandler(t *testing.T) {
	ch := testutils.NewServer(t, nil)
	defer ch.Close()

	var someHandler HandlerFunc = func(ctx context.Context, call *InboundCall) {
		panic("unexpected call")
	}
	var anotherHandler HandlerFunc = func(ctx context.Context, call *InboundCall) {
		panic("unexpected call")
	}

	ch.GetSubChannel("foo").SetHandler(someHandler)

	// Registering against the original service should not panic but
	// registering against the "foo" service should panic.
	assert.NotPanics(t, func() { ch.Register(anotherHandler, "bar") })
	assert.NotPanics(t, func() { ch.GetSubChannel("svc").GetHandlers() })
	assert.Panics(t, func() { ch.GetSubChannel("foo").Register(anotherHandler, "bar") })
	assert.Panics(t, func() { ch.GetSubChannel("foo").GetHandlers() })
}

func TestGetSubchannelOptionsOnNew(t *testing.T) {
	ch := testutils.NewServer(t, nil)
	defer ch.Close()

	peers := ch.GetSubChannel("s", Isolated).Peers()
	want := peers.Add("1.1.1.1:1")

	peers2 := ch.GetSubChannel("s", Isolated).Peers()
	assert.Equal(t, peers, peers2, "Get isolated subchannel should not clear existing peers")
	peer, err := peers2.Get(nil)
	require.NoError(t, err, "Should get peer")
	assert.Equal(t, want, peer, "Unexpected peer")
}

func TestHandlerWithoutSubChannel(t *testing.T) {
	opts := testutils.NewOpts().NoRelay()
	opts.Handler = raw.Wrap(newTestHandler(t))
	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		client := ts.NewClient(nil)
		testutils.AssertEcho(t, client, ts.HostPort(), ts.ServiceName())
		testutils.AssertEcho(t, client, ts.HostPort(), "larry")
		testutils.AssertEcho(t, client, ts.HostPort(), "curly")
		testutils.AssertEcho(t, client, ts.HostPort(), "moe")

		assert.Panics(t, func() {
			ts.Server().Register(raw.Wrap(newTestHandler(t)), "nyuck")
		})
	})
}
