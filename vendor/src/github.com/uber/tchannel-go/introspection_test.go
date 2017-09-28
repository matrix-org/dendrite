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
	"github.com/uber/tchannel-go/json"
	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Purpose of this test is to ensure introspection doesn't cause any panics
// and we have coverage of the introspection code.
func TestIntrospection(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		client := testutils.NewClient(t, nil)
		defer client.Close()

		ctx, cancel := json.NewContext(time.Second)
		defer cancel()

		var resp map[string]interface{}
		peer := client.Peers().GetOrAdd(ts.HostPort())
		err := json.CallPeer(ctx, peer, ts.ServiceName(), "_gometa_introspect", map[string]interface{}{
			"includeExchanges":  true,
			"includeEmptyPeers": true,
			"includeTombstones": true,
		}, &resp)
		require.NoError(t, err, "Call _gometa_introspect failed")

		err = json.CallPeer(ctx, peer, ts.ServiceName(), "_gometa_runtime", map[string]interface{}{
			"includeGoStacks": true,
		}, &resp)
		require.NoError(t, err, "Call _gometa_runtime failed")

		if !ts.HasRelay() {
			// Try making the call on the "tchannel" service which is where meta handlers
			// are registered. This will only work when we call it directly as the relay
			// will not forward the tchannel service.
			err = json.CallPeer(ctx, peer, "tchannel", "_gometa_runtime", map[string]interface{}{
				"includeGoStacks": true,
			}, &resp)
			require.NoError(t, err, "Call _gometa_runtime failed")
		}
	})
}

func TestIntrospectNumConnections(t *testing.T) {
	// Disable the relay, since the relay does not maintain a 1:1 mapping betewen
	// incoming connections vs outgoing connections.
	opts := testutils.NewOpts().NoRelay()
	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		ctx, cancel := NewContext(time.Second)
		defer cancel()

		assert.Equal(t, 0, ts.Server().IntrospectNumConnections(), "Expected no connection on new server")

		for i := 0; i < 10; i++ {
			client := ts.NewClient(nil)
			defer client.Close()

			require.NoError(t, client.Ping(ctx, ts.HostPort()), "Ping from new client failed")
			assert.Equal(t, 1, client.IntrospectNumConnections(), "Client should have single connection")
			assert.Equal(t, i+1, ts.Server().IntrospectNumConnections(), "Incorrect number of server connections")
		}

		// Make sure that a closed connection will reduce NumConnections.
		client := ts.NewClient(nil)
		require.NoError(t, client.Ping(ctx, ts.HostPort()), "Ping from new client failed")
		assert.Equal(t, 11, ts.Server().IntrospectNumConnections(), "Number of connections expected to increase")

		client.Close()
		require.True(t, testutils.WaitFor(100*time.Millisecond, func() bool {
			return ts.Server().IntrospectNumConnections() == 10
		}), "Closed connection did not get removed, num connections is %v", ts.Server().IntrospectNumConnections())
	})
}
