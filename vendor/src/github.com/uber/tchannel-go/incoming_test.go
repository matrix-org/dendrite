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

	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/assert"
)

func TestPeersIncomingConnection(t *testing.T) {
	newService := func(svcName string) (*Channel, string) {
		ch, _, hostPort := NewServer(t, &testutils.ChannelOpts{ServiceName: svcName})
		return ch, hostPort
	}

	opts := testutils.NewOpts().NoRelay()
	WithVerifiedServer(t, opts, func(ch *Channel, hostPort string) {
		doPing := func(ch *Channel) {
			ctx, cancel := NewContext(time.Second)
			defer cancel()
			assert.NoError(t, ch.Ping(ctx, hostPort), "Ping failed")
		}

		hyperbahnSC := ch.GetSubChannel("hyperbahn")
		ringpopSC := ch.GetSubChannel("ringpop", Isolated)

		hyperbahn, hyperbahnHostPort := newService("hyperbahn")
		defer hyperbahn.Close()
		ringpop, ringpopHostPort := newService("ringpop")
		defer ringpop.Close()

		doPing(hyperbahn)
		doPing(ringpop)

		// The root peer list should contain all incoming connections.
		rootPeers := ch.RootPeers().Copy()
		assert.NotNil(t, rootPeers[hyperbahnHostPort], "missing hyperbahn peer")
		assert.NotNil(t, rootPeers[ringpopHostPort], "missing ringpop peer")

		for _, sc := range []Registrar{ch, hyperbahnSC, ringpopSC} {
			_, err := sc.Peers().Get(nil)
			assert.Equal(t, ErrNoPeers, err,
				"incoming connections should not be added to non-root peer list")
		}

		// verify number of peers/connections on the client side
		serverState := ch.IntrospectState(nil).RootPeers
		serverHostPort := ch.PeerInfo().HostPort

		assert.Equal(t, len(serverState), 2, "Incorrect peer count")
		for _, client := range []*Channel{ringpop, hyperbahn} {
			clientPeerState := client.IntrospectState(nil).RootPeers
			clientHostPort := client.PeerInfo().HostPort
			assert.Equal(t, len(clientPeerState), 1, "Incorrect peer count")
			assert.Equal(t, len(clientPeerState[serverHostPort].OutboundConnections), 1, "Incorrect outbound connection count")
			assert.Equal(t, len(clientPeerState[serverHostPort].InboundConnections), 0, "Incorrect inbound connection count")

			assert.Equal(t, len(serverState[clientHostPort].InboundConnections), 1, "Incorrect inbound connection count")
			assert.Equal(t, len(serverState[clientHostPort].OutboundConnections), 0, "Incorrect outbound connection count")
		}

		// In future when connections send a service name, we should be able to
		// check that a new connection containing a service name for an isolated
		// subchannel is only added to the isolated subchannels' peers, but all
		// other incoming connections are added to the shared peer list.
	})
}
