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

package json

import (
	"fmt"
	"testing"
	"time"

	"github.com/uber/tchannel-go"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

// ForwardArgs are the arguments specifying who to forward to (and the message to forward).
type ForwardArgs struct {
	HeaderVal   string
	Service     string
	Method      string
	NextForward *ForwardArgs
}

// Res is the final result.
type Res struct {
	Result string
}

type testHandler struct {
	calls   []string
	callers []string
	peer    *tchannel.Peer
	t       *testing.T
}

func (h *testHandler) forward(ctx Context, args *ForwardArgs) (*Res, error) {
	headerVal := ctx.Headers()["hdr"]
	ctx.SetResponseHeaders(map[string]string{"hdr": headerVal + "-resp"})
	h.calls = append(h.calls, "forward-"+headerVal)
	h.callers = append(h.callers, tchannel.CurrentCall(ctx).CallerName())

	if args.HeaderVal != "" {
		ctx = WithHeaders(ctx, map[string]string{"hdr": args.HeaderVal})
	}
	res := &Res{}

	if args.Method == "forward" {
		if err := CallPeer(ctx, h.peer, args.Service, args.Method, args.NextForward, res); err != nil {
			h.t.Errorf("forward->forward Call failed: %v", err)
			return nil, err
		}
		assert.Equal(h.t, map[string]string{"hdr": args.HeaderVal + "-resp"}, ctx.ResponseHeaders())
		return res, nil
	}

	if err := CallPeer(ctx, h.peer, args.Service, args.Method, nil, res); err != nil {
		h.t.Errorf("forward->%v Call failed: %v", args.Method, err)
		return nil, err
	}

	return res, nil
}

func (h *testHandler) leaf(ctx Context, _ *struct{}) (*Res, error) {
	headerVal := ctx.Headers()["hdr"]
	h.calls = append(h.calls, "leaf-"+headerVal)
	h.callers = append(h.callers, tchannel.CurrentCall(ctx).CallerName())
	return &Res{"leaf called!"}, nil
}

func (h *testHandler) onError(ctx context.Context, err error) {
	h.t.Errorf("onError(%v)", err)
}

func TestForwardChain(t *testing.T) {
	servers := map[string]*struct {
		channel   *tchannel.Channel
		handler   *testHandler
		otherPeer string
	}{
		"serv1": {otherPeer: "serv2"},
		"serv2": {otherPeer: "serv3"},
		"serv3": {otherPeer: "serv1"},
	}

	// We want the following call graph:
	// serv1.forward
	// -> (1) serv2.forward
	// -> (2) serv3.forward
	// -> (3) serv1.forward
	// -> (4) serv2.forward
	// ....
	// -> (11) serv3.leaf
	rootArg := &ForwardArgs{}
	curArg := rootArg
	for i := 1; i <= 10; i++ {
		service := fmt.Sprintf("serv%v", (i%3)+1)

		curArg.Method = "forward"
		curArg.HeaderVal = fmt.Sprint(i)
		curArg.Service = service
		curArg.NextForward = &ForwardArgs{}

		curArg = curArg.NextForward
	}
	curArg.Service = "serv3"
	curArg.HeaderVal = "11"
	curArg.Method = "leaf"

	expectedCalls := map[string]struct {
		calls   []string
		callers []string
	}{
		"serv1": {
			calls:   []string{"forward-initial", "forward-3", "forward-6", "forward-9"},
			callers: []string{"serv3", "serv3", "serv3", "serv3"},
		},
		"serv2": {
			calls:   []string{"forward-1", "forward-4", "forward-7", "forward-10"},
			callers: []string{"serv1", "serv1", "serv1", "serv1"},
		},
		"serv3": {
			calls:   []string{"forward-2", "forward-5", "forward-8", "leaf-11"},
			callers: []string{"serv2", "serv2", "serv2", "serv2"},
		},
	}

	// Use the above data to setup the test and ensure the calls are made as expected.
	for name, s := range servers {
		var err error
		s.channel, err = tchannel.NewChannel(name, nil)
		require.NoError(t, err)

		s.handler = &testHandler{t: t}
		require.NoError(t, Register(s.channel, Handlers{
			"forward": s.handler.forward,
			"leaf":    s.handler.leaf,
		}, s.handler.onError))

		require.NoError(t, s.channel.ListenAndServe("127.0.0.1:0"))
	}
	for _, s := range servers {
		s.handler.peer = s.channel.Peers().Add(servers[s.otherPeer].channel.PeerInfo().HostPort)
	}

	ctx, cancel := NewContext(time.Second)
	defer cancel()
	ctx = WithHeaders(ctx, map[string]string{"hdr": "initial"})
	assert.Nil(t, tchannel.CurrentCall(ctx))

	sc := servers["serv3"].channel.GetSubChannel("serv1")
	resp := &Res{}
	if assert.NoError(t, CallSC(ctx, sc, "forward", rootArg, resp)) {
		assert.Equal(t, "leaf called!", resp.Result)
		for s, expected := range expectedCalls {
			assert.Equal(t, expected.calls, servers[s].handler.calls, "wrong calls for %v", s)
			assert.Equal(t, expected.callers, servers[s].handler.callers, "wrong callers for %v", s)
		}
	}
}

func TestHeadersForwarded(t *testing.T) {
	ch, err := tchannel.NewChannel("svc", nil)
	require.NoError(t, err)

	handler := &testHandler{t: t}
	require.NoError(t, Register(ch, Handlers{
		"forward": handler.forward,
		"leaf":    handler.leaf,
	}, handler.onError))
	assert.NoError(t, ch.ListenAndServe("127.0.0.1:0"))

	rootArg := &ForwardArgs{
		Service:   "svc",
		Method:    "leaf",
		HeaderVal: "",
	}

	ctx, cancel := NewContext(time.Second)
	defer cancel()
	ctx = WithHeaders(ctx, map[string]string{"hdr": "copy"})

	assert.Nil(t, tchannel.CurrentCall(ctx))
	resp := &Res{}
	handler.peer = ch.Peers().Add(ch.PeerInfo().HostPort)
	if assert.NoError(t, CallPeer(ctx, handler.peer, "svc", "forward", rootArg, resp)) {
		// Verify that the header is copied when ctx is not changed.
		assert.Equal(t, handler.calls, []string{"forward-copy", "leaf-copy"})
	}
}

func TestEmptyRequestHeader(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	ch, err := tchannel.NewChannel("server", nil)
	require.NoError(t, err)
	require.NoError(t, ch.ListenAndServe("127.0.0.1:0"))

	handler := func(ctx Context, _ *struct{}) (*struct{}, error) {
		assert.Equal(t, map[string]string(nil), ctx.Headers())
		return nil, nil
	}
	onError := func(ctx context.Context, err error) {
		t.Errorf("onError: %v", err)
	}
	require.NoError(t, Register(ch, Handlers{"handle": handler}, onError))

	call, err := ch.BeginCall(ctx, ch.PeerInfo().HostPort, "server", "handle", &tchannel.CallOptions{
		Format: tchannel.JSON,
	})
	require.NoError(t, err)

	require.NoError(t, tchannel.NewArgWriter(call.Arg2Writer()).Write(nil))
	require.NoError(t, tchannel.NewArgWriter(call.Arg3Writer()).WriteJSON(nil))

	resp := call.Response()
	var data interface{}
	require.NoError(t, tchannel.NewArgReader(resp.Arg2Reader()).ReadJSON(&data))
	require.NoError(t, tchannel.NewArgReader(resp.Arg3Reader()).ReadJSON(&data))
}

func TestMapInputOutput(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	ch, err := tchannel.NewChannel("server", nil)
	require.NoError(t, err)
	require.NoError(t, ch.ListenAndServe("127.0.0.1:0"))

	handler := func(ctx Context, args map[string]interface{}) (map[string]interface{}, error) {
		return args, nil
	}
	onError := func(ctx context.Context, err error) {
		t.Errorf("onError: %v", err)
	}
	require.NoError(t, Register(ch, Handlers{"handle": handler}, onError))

	call, err := ch.BeginCall(ctx, ch.PeerInfo().HostPort, "server", "handle", &tchannel.CallOptions{
		Format: tchannel.JSON,
	})
	require.NoError(t, err)

	arg := map[string]interface{}{
		"v1": "value1",
		"v2": 2.0,
		"v3": map[string]interface{}{"k": "v", "k2": "v2"},
	}
	require.NoError(t, tchannel.NewArgWriter(call.Arg2Writer()).Write(nil))
	require.NoError(t, tchannel.NewArgWriter(call.Arg3Writer()).WriteJSON(arg))

	resp := call.Response()
	var data interface{}
	require.NoError(t, tchannel.NewArgReader(resp.Arg2Reader()).ReadJSON(&data))
	require.NoError(t, tchannel.NewArgReader(resp.Arg3Reader()).ReadJSON(&data))
	assert.Equal(t, arg, data.(map[string]interface{}), "result does not match arg")
}
