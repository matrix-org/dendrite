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

package mockhyperbahn_test

import (
	"testing"
	"time"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/hyperbahn"
	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"
	"github.com/uber/tchannel-go/testutils/mockhyperbahn"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/atomic"
)

var config = struct {
	hyperbahnConfig hyperbahn.Configuration
}{}

// setupServer is the application code we are attempting to test.
func setupServer() (*hyperbahn.Client, error) {
	ch, err := tchannel.NewChannel("myservice", nil)
	if err != nil {
		return nil, err
	}

	if err := ch.ListenAndServe("127.0.0.1:0"); err != nil {
		return nil, err
	}

	client, err := hyperbahn.NewClient(ch, config.hyperbahnConfig, nil)
	if err != nil {
		return nil, err
	}

	return client, client.Advertise()
}

func newAdvertisedEchoServer(t *testing.T, name string, mockHB *mockhyperbahn.Mock, f func()) *tchannel.Channel {
	server := testutils.NewServer(t, &testutils.ChannelOpts{
		ServiceName: name,
	})
	testutils.RegisterEcho(server, f)

	hbClient, err := hyperbahn.NewClient(server, mockHB.Configuration(), nil)
	require.NoError(t, err, "Failed to set up Hyperbahn client")
	require.NoError(t, hbClient.Advertise(), "Advertise failed")

	return server
}

func TestMockHyperbahn(t *testing.T) {
	mh, err := mockhyperbahn.New()
	require.NoError(t, err, "mock hyperbahn failed")
	defer mh.Close()

	config.hyperbahnConfig = mh.Configuration()
	_, err = setupServer()
	require.NoError(t, err, "setupServer failed")
	assert.Equal(t, []string{"myservice"}, mh.GetAdvertised())
}

func TestMockDiscovery(t *testing.T) {
	mh, err := mockhyperbahn.New()
	require.NoError(t, err, "mock hyperbahn failed")
	defer mh.Close()

	peers := []string{
		"1.3.5.7:1456",
		"255.255.255.255:25",
	}
	mh.SetDiscoverResult("discover-svc", peers)

	config.hyperbahnConfig = mh.Configuration()
	client, err := setupServer()
	require.NoError(t, err, "setupServer failed")

	gotPeers, err := client.Discover("discover-svc")
	require.NoError(t, err, "Discover failed")
	assert.Equal(t, peers, gotPeers, "Discover returned invalid peers")
}

func TestMockForwards(t *testing.T) {
	mockHB, err := mockhyperbahn.New()
	require.NoError(t, err, "Failed to set up mock hyperbahm")

	called := false
	server := newAdvertisedEchoServer(t, "svr", mockHB, func() {
		called = true
	})
	defer server.Close()
	client := newAdvertisedEchoServer(t, "client", mockHB, nil)
	defer client.Close()

	ctx, cancel := tchannel.NewContext(time.Second)
	defer cancel()

	_, _, _, err = raw.CallSC(ctx, client.GetSubChannel("svr"), "echo", nil, nil)
	require.NoError(t, err, "Call failed")
	require.True(t, called, "Advertised server was not called")
}

func TestMockIgnoresDown(t *testing.T) {
	mockHB, err := mockhyperbahn.New()
	require.NoError(t, err, "Failed to set up mock hyperbahm")

	var (
		moe1Called atomic.Bool
		moe2Called atomic.Bool
	)

	moe1 := newAdvertisedEchoServer(t, "moe", mockHB, func() { moe1Called.Store(true) })
	defer moe1.Close()
	moe2 := newAdvertisedEchoServer(t, "moe", mockHB, func() { moe2Called.Store(true) })
	defer moe2.Close()
	client := newAdvertisedEchoServer(t, "client", mockHB, nil)

	ctx, cancel := tchannel.NewContext(time.Second)
	defer cancel()

	for i := 0; i < 20; i++ {
		_, _, _, err = raw.CallSC(ctx, client.GetSubChannel("moe"), "echo", nil, nil)
		assert.NoError(t, err, "Call failed")
	}

	require.True(t, moe1Called.Load(), "moe1 not called")
	require.True(t, moe2Called.Load(), "moe2 not called")

	// If moe2 is brought down, all calls should now be sent to moe1.
	moe2.Close()

	// Wait for the mock HB to have 0 connections to moe
	ok := testutils.WaitFor(time.Second, func() bool {
		in, out := mockHB.Channel().Peers().GetOrAdd(moe2.PeerInfo().HostPort).NumConnections()
		return in+out == 0
	})
	require.True(t, ok, "Failed waiting for mock HB to have 0 connections")

	// Make sure that all calls succeed (they should all go to moe2)
	moe1Called.Store(false)
	moe2Called.Store(false)
	for i := 0; i < 20; i++ {
		_, _, _, err = raw.CallSC(ctx, client.GetSubChannel("moe"), "echo", nil, nil)
		assert.NoError(t, err, "Call failed")
	}

	require.True(t, moe1Called.Load(), "moe1 not called")
	require.False(t, moe2Called.Load(), "moe2 should not be called after Close")
}
