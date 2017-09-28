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
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	. "github.com/uber/tchannel-go"

	"github.com/uber/tchannel-go/benchmark"
	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/atomic"
)

func fakePeer(t *testing.T, ch *Channel, hostPort string) *Peer {
	ch.Peers().Add(hostPort)

	peer, err := ch.Peers().Get(nil)
	require.NoError(t, err, "Unexpected error getting peer from heap.")
	require.Equal(t, hostPort, peer.HostPort(), "Got unexpected peer.")

	in, out := peer.NumConnections()
	require.Equal(t, 0, in, "Expected new peer to have no incoming connections.")
	require.Equal(t, 0, out, "Expected new peer to have no outgoing connections.")

	return peer
}

func assertNumConnections(t *testing.T, peer *Peer, in, out int) {
	actualIn, actualOut := peer.NumConnections()
	assert.Equal(t, actualIn, in, "Expected %v incoming connection.", in)
	assert.Equal(t, actualOut, out, "Expected %v outgoing connection.", out)
}

func TestGetPeerNoPeer(t *testing.T) {
	ch := testutils.NewClient(t, nil)
	defer ch.Close()
	peer, err := ch.Peers().Get(nil)
	assert.Equal(t, ErrNoPeers, err, "Empty peer list should return error")
	assert.Nil(t, peer, "should not return peer")
}

func TestGetPeerSinglePeer(t *testing.T) {
	ch := testutils.NewClient(t, nil)
	defer ch.Close()
	ch.Peers().Add("1.1.1.1:1234")

	peer, err := ch.Peers().Get(nil)
	assert.NoError(t, err, "peer list should return contained element")
	assert.Equal(t, "1.1.1.1:1234", peer.HostPort(), "returned peer mismatch")
}

func TestPeerUpdatesLen(t *testing.T) {
	ch := testutils.NewClient(t, nil)
	defer ch.Close()
	assert.Zero(t, ch.Peers().Len())
	for i := 1; i < 5; i++ {
		ch.Peers().Add(fmt.Sprintf("1.1.1.1:%d", i))
		assert.Equal(t, ch.Peers().Len(), i)
	}
	for i := 4; i > 0; i-- {
		assert.Equal(t, ch.Peers().Len(), i)
		ch.Peers().Remove(fmt.Sprintf("1.1.1.1:%d", i))
	}
	assert.Zero(t, ch.Peers().Len())
}

func TestGetPeerAvoidPrevSelected(t *testing.T) {
	const (
		peer1 = "1.1.1.1:1"
		peer2 = "2.2.2.2:2"
		peer3 = "3.3.3.3:3"
		peer4 = "3.3.3.3:4"
	)

	ch := testutils.NewClient(t, nil)
	defer ch.Close()
	a, m := testutils.StrArray, testutils.StrMap
	tests := []struct {
		msg          string
		peers        []string
		prevSelected []string
		expected     map[string]struct{}
	}{
		{
			msg:      "no prevSelected",
			peers:    a(peer1),
			expected: m(peer1),
		},
		{
			msg:          "ignore single hostPort in prevSelected",
			peers:        a(peer1, peer2),
			prevSelected: a(peer1),
			expected:     m(peer2),
		},
		{
			msg:          "ignore multiple hostPorts in prevSelected",
			peers:        a(peer1, peer2, peer3),
			prevSelected: a(peer1, peer2),
			expected:     m(peer3),
		},
		{
			msg:          "only peer is in prevSelected",
			peers:        a(peer1),
			prevSelected: a(peer1),
			expected:     m(peer1),
		},
		{
			msg:          "all peers are in prevSelected",
			peers:        a(peer1, peer2, peer3),
			prevSelected: a(peer1, peer2, peer3),
			expected:     m(peer1, peer2, peer3),
		},
		{
			msg:          "prevSelected host should be ignored",
			peers:        a(peer1, peer3, peer4),
			prevSelected: a(peer3),
			expected:     m(peer1),
		},
		{
			msg:          "prevSelected only has single host",
			peers:        a(peer3, peer4),
			prevSelected: a(peer3),
			expected:     m(peer4),
		},
	}

	for i, tt := range tests {
		peers := ch.GetSubChannel(fmt.Sprintf("test%d", i), Isolated).Peers()
		for _, p := range tt.peers {
			peers.Add(p)
		}

		rs := &RequestState{}
		for _, selected := range tt.prevSelected {
			rs.AddSelectedPeer(selected)
		}

		gotPeer, err := peers.Get(rs.PrevSelectedPeers())
		if err != nil {
			t.Errorf("Got unexpected error selecting peer: %v", err)
			continue
		}

		newPeer, err := peers.GetNew(rs.PrevSelectedPeers())
		if len(tt.peers) == len(tt.prevSelected) {
			if newPeer != nil || err != ErrNoNewPeers {
				t.Errorf("%s: newPeer should not have been found %v: %v\n", tt.msg, newPeer, err)
			}
		} else {
			if gotPeer != newPeer || err != nil {
				t.Errorf("%s: expected equal peers, got %v new %v: %v\n",
					tt.msg, gotPeer, newPeer, err)
			}
		}

		got := gotPeer.HostPort()
		if _, ok := tt.expected[got]; !ok {
			t.Errorf("%s: got unexpected peer, expected one of %v got %v\n  Peers = %v PrevSelected = %v",
				tt.msg, tt.expected, got, tt.peers, tt.prevSelected)
		}
	}
}

func TestPeerRemoveClosedConnection(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	WithVerifiedServer(t, nil, func(ch *Channel, hostPort string) {
		client := testutils.NewClient(t, nil)
		defer client.Close()

		p := client.Peers().Add(hostPort)

		c1, err := p.Connect(ctx)
		require.NoError(t, err, "Failed to connect")
		require.NoError(t, err, c1.Ping(ctx))

		c2, err := p.Connect(ctx)
		require.NoError(t, err, "Failed to connect")
		require.NoError(t, err, c2.Ping(ctx))

		require.NoError(t, c1.Close(), "Failed to close first connection")
		_, outConns := p.NumConnections()
		assert.Equal(t, 1, outConns, "Expected 1 remaining outgoing connection")

		c, err := p.GetConnection(ctx)
		require.NoError(t, err, "GetConnection failed")
		assert.Equal(t, c2, c, "Expected second active connection")
	})
}

func TestPeerConnectCancelled(t *testing.T) {
	WithVerifiedServer(t, nil, func(ch *Channel, hostPort string) {
		ctx, cancel := NewContext(100 * time.Millisecond)
		cancel()

		_, err := ch.Connect(ctx, "10.255.255.1:1")
		require.Error(t, err, "Connect should fail")
		assert.EqualError(t, err, ErrRequestCancelled.Error(), "Unexpected error")
	})
}

func TestPeerGetConnectionWithNoActiveConnections(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	WithVerifiedServer(t, nil, func(ch *Channel, hostPort string) {
		client := testutils.NewClient(t, nil)
		defer client.Close()

		var (
			wg          sync.WaitGroup
			lock        sync.Mutex
			conn        *Connection
			concurrency = 10
			p           = client.Peers().Add(hostPort)
		)

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				c, err := p.GetConnection(ctx)
				require.NoError(t, err, "GetConnection failed")

				lock.Lock()
				defer lock.Unlock()

				if conn == nil {
					conn = c
				} else {
					assert.Equal(t, conn, c, "Expected the same active connection")
				}

			}()
		}

		wg.Wait()

		_, outbound := p.NumConnections()
		assert.Equal(t, 1, outbound, "Expected 1 active outbound connetion")
	})
}

func TestInboundEphemeralPeerRemoved(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	// No relay, since we look for the exact host:port in peer lists.
	opts := testutils.NewOpts().NoRelay()
	WithVerifiedServer(t, opts, func(ch *Channel, hostPort string) {
		client := testutils.NewClient(t, nil)
		assert.NoError(t, client.Ping(ctx, hostPort), "Ping to server failed")

		// Server should have a host:port in the root peers for the client.
		var clientHP string
		peers := ch.RootPeers().Copy()
		for k := range peers {
			clientHP = k
		}

		waitTillInboundEmpty(t, ch, clientHP, func() {
			client.Close()
		})
		assert.Equal(t, ChannelClosed, client.State(), "Client should be closed")

		_, ok := ch.RootPeers().Get(clientHP)
		assert.False(t, ok, "server's root peers should remove peer for client on close")
	})
}

func TestOutboundEphemeralPeerRemoved(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	WithVerifiedServer(t, nil, func(ch *Channel, hostPort string) {
		outbound := testutils.NewServer(t, testutils.NewOpts().SetServiceName("asd	"))
		assert.NoError(t, ch.Ping(ctx, outbound.PeerInfo().HostPort), "Ping to outbound failed")
		outboundHP := outbound.PeerInfo().HostPort

		// Server should have a peer for hostPort that should be gone.
		waitTillNConnections(t, ch, outboundHP, 0, 0, func() {
			outbound.Close()
		})
		assert.Equal(t, ChannelClosed, outbound.State(), "Outbound should be closed")

		_, ok := ch.RootPeers().Get(outboundHP)
		assert.False(t, ok, "server's root peers should remove outbound peer")
	})
}

func TestOutboundPeerNotAdded(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	WithVerifiedServer(t, nil, func(server *Channel, hostPort string) {
		server.Register(raw.Wrap(newTestHandler(t)), "echo")

		ch := testutils.NewClient(t, nil)
		defer ch.Close()

		ch.Ping(ctx, hostPort)
		raw.Call(ctx, ch, hostPort, server.PeerInfo().ServiceName, "echo", nil, nil)

		peer, err := ch.Peers().Get(nil)
		assert.Equal(t, ErrNoPeers, err, "Ping should not add peers")
		assert.Nil(t, peer, "Expected no peer to be returned")
	})
}

func TestRemovePeerNotFound(t *testing.T) {
	ch := testutils.NewClient(t, nil)
	defer ch.Close()

	peers := ch.Peers()
	peers.Add("1.1.1.1:1")
	assert.Error(t, peers.Remove("not-found"), "Remove should fa")
	assert.NoError(t, peers.Remove("1.1.1.1:1"), "Remove shouldn't fail for existing peer")
}

func TestPeerRemovedFromRootPeers(t *testing.T) {
	tests := []struct {
		addHostPort    bool
		removeHostPort bool
		expectFound    bool
	}{
		{
			addHostPort: true,
			expectFound: true,
		},
		{
			addHostPort:    true,
			removeHostPort: true,
			expectFound:    false,
		},
		{
			addHostPort: false,
			expectFound: false,
		},
	}

	ctx, cancel := NewContext(time.Second)
	defer cancel()

	for _, tt := range tests {
		opts := testutils.NewOpts().NoRelay()
		WithVerifiedServer(t, opts, func(server *Channel, hostPort string) {
			ch := testutils.NewServer(t, nil)
			clientHP := ch.PeerInfo().HostPort

			if tt.addHostPort {
				server.Peers().Add(clientHP)
			}

			assert.NoError(t, ch.Ping(ctx, hostPort), "Ping failed")

			if tt.removeHostPort {
				require.NoError(t, server.Peers().Remove(clientHP), "Failed to remove peer")
			}

			waitTillInboundEmpty(t, server, clientHP, func() {
				ch.Close()
			})

			rootPeers := server.RootPeers()
			_, found := rootPeers.Get(clientHP)
			assert.Equal(t, tt.expectFound, found, "Peer found mismatch, addHostPort: %v", tt.addHostPort)
		})
	}
}

func TestPeerSelectionConnClosed(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	WithVerifiedServer(t, nil, func(server *Channel, hostPort string) {
		client := testutils.NewServer(t, nil)
		defer client.Close()

		// Ping will create an outbound connection from client -> server.
		require.NoError(t, testutils.Ping(client, server), "Ping failed")

		waitTillInboundEmpty(t, server, client.PeerInfo().HostPort, func() {
			peer, ok := client.RootPeers().Get(server.PeerInfo().HostPort)
			require.True(t, ok, "Client has no peer for %v", server.PeerInfo())

			conn, err := peer.GetConnection(ctx)
			require.NoError(t, err, "Failed to get a connection")
			conn.Close()
		})

		// Make sure the closed connection is not used.
		for i := 0; i < 10; i++ {
			require.NoError(t, testutils.Ping(client, server), "Ping failed")
		}
	})
}

func TestPeerSelectionPreferIncoming(t *testing.T) {
	tests := []struct {
		numIncoming, numOutgoing, numUnconnected int
		isolated                                 bool
		expectedIncoming                         int
		expectedOutgoing                         int
		expectedUnconnected                      int
	}{
		{
			numIncoming:      5,
			numOutgoing:      5,
			numUnconnected:   5,
			expectedIncoming: 5,
		},
		{
			numOutgoing:      5,
			numUnconnected:   5,
			expectedOutgoing: 5,
		},
		{
			numUnconnected:      5,
			expectedUnconnected: 5,
		},
		{
			numIncoming:      5,
			numOutgoing:      5,
			numUnconnected:   5,
			isolated:         true,
			expectedIncoming: 5,
			expectedOutgoing: 5,
		},
		{
			numOutgoing:      5,
			numUnconnected:   5,
			isolated:         true,
			expectedOutgoing: 5,
		},
		{
			numIncoming:      5,
			numUnconnected:   5,
			isolated:         true,
			expectedIncoming: 5,
		},
		{
			numUnconnected:      5,
			isolated:            true,
			expectedUnconnected: 5,
		},
	}

	for _, tt := range tests {
		// We need to directly connect from the server to the client and verify
		// the exact peers.
		opts := testutils.NewOpts().NoRelay()
		WithVerifiedServer(t, opts, func(ch *Channel, hostPort string) {
			ctx, cancel := NewContext(time.Second)
			defer cancel()

			selectedIncoming := make(map[string]int)
			selectedOutgoing := make(map[string]int)
			selectedUnconnected := make(map[string]int)

			peers := ch.Peers()
			if tt.isolated {
				peers = ch.GetSubChannel("isolated", Isolated).Peers()
			}

			// 5 peers that make incoming connections to ch.
			for i := 0; i < tt.numIncoming; i++ {
				incoming, _, incomingHP := NewServer(t, &testutils.ChannelOpts{ServiceName: fmt.Sprintf("incoming%d", i)})
				defer incoming.Close()
				assert.NoError(t, incoming.Ping(ctx, ch.PeerInfo().HostPort), "Ping failed")
				peers.Add(incomingHP)
				selectedIncoming[incomingHP] = 0
			}

			// 5 random peers that don't have any connections.
			for i := 0; i < tt.numUnconnected; i++ {
				hp := fmt.Sprintf("1.1.1.1:1%d", i)
				peers.Add(hp)
				selectedUnconnected[hp] = 0
			}

			// 5 random peers that we have outgoing connections to.
			for i := 0; i < tt.numOutgoing; i++ {
				outgoing, _, outgoingHP := NewServer(t, &testutils.ChannelOpts{ServiceName: fmt.Sprintf("outgoing%d", i)})
				defer outgoing.Close()
				assert.NoError(t, ch.Ping(ctx, outgoingHP), "Ping failed")
				peers.Add(outgoingHP)
				selectedOutgoing[outgoingHP] = 0
			}

			var mu sync.Mutex
			checkMap := func(m map[string]int, peer string) bool {
				mu.Lock()
				defer mu.Unlock()

				if _, ok := m[peer]; !ok {
					return false
				}

				m[peer]++
				return true
			}

			numSelectedPeers := func(m map[string]int) int {
				count := 0
				for _, v := range m {
					if v > 0 {
						count++
					}
				}
				return count
			}

			peerCheck := func() {
				for i := 0; i < 100; i++ {
					peer, err := peers.Get(nil)
					if assert.NoError(t, err, "Peers.Get failed") {
						peerHP := peer.HostPort()
						inMap := checkMap(selectedIncoming, peerHP) ||
							checkMap(selectedOutgoing, peerHP) ||
							checkMap(selectedUnconnected, peerHP)
						assert.True(t, inMap, "Couldn't find peer %v in any of our maps", peerHP)
					}
				}
			}

			// Now select peers in parallel
			var wg sync.WaitGroup
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					peerCheck()
				}()
			}
			wg.Wait()

			assert.Equal(t, tt.expectedIncoming, numSelectedPeers(selectedIncoming),
				"Selected incoming mismatch: %v", selectedIncoming)
			assert.Equal(t, tt.expectedOutgoing, numSelectedPeers(selectedOutgoing),
				"Selected outgoing mismatch: %v", selectedOutgoing)
			assert.Equal(t, tt.expectedUnconnected, numSelectedPeers(selectedUnconnected),
				"Selected unconnected mismatch: %v", selectedUnconnected)
		})
	}
}

type peerTest struct {
	t        testing.TB
	channels []*Channel
}

// NewService will return a new server channel and the host port.
func (pt *peerTest) NewService(t testing.TB, svcName, processName string) (*Channel, string) {
	opts := testutils.NewOpts().SetServiceName(svcName).SetProcessName(processName)
	ch := testutils.NewServer(t, opts)
	pt.channels = append(pt.channels, ch)
	return ch, ch.PeerInfo().HostPort
}

// CleanUp will clean up all channels started as part of the peer test.
func (pt *peerTest) CleanUp() {
	for _, ch := range pt.channels {
		ch.Close()
	}
}

func TestPeerSelection(t *testing.T) {
	pt := &peerTest{t: t}
	defer pt.CleanUp()
	WithVerifiedServer(t, &testutils.ChannelOpts{ServiceName: "S1"}, func(ch *Channel, hostPort string) {
		doPing := func(ch *Channel) {
			ctx, cancel := NewContext(time.Second)
			defer cancel()
			assert.NoError(t, ch.Ping(ctx, hostPort), "Ping failed")
		}

		strategy, count := createScoreStrategy(0, 1)
		s2, _ := pt.NewService(t, "S2", "S2")
		defer s2.Close()

		s2.GetSubChannel("S1").Peers().SetStrategy(strategy)
		s2.GetSubChannel("S1").Peers().Add(hostPort)
		doPing(s2)
		assert.EqualValues(t, 4, count.Load(),
			"Expect 4 exchange updates: peer add, new conn, ping, pong")
	})
}

func getAllPeers(t *testing.T, pl *PeerList) []string {
	prevSelected := make(map[string]struct{})
	var got []string

	for {
		peer, err := pl.Get(prevSelected)
		require.NoError(t, err, "Peer.Get failed")

		hp := peer.HostPort()
		if _, ok := prevSelected[hp]; ok {
			break
		}

		prevSelected[hp] = struct{}{}
		got = append(got, hp)
	}

	return got
}

func reverse(s []string) {
	for i := 0; i < len(s)/2; i++ {
		j := len(s) - i - 1
		s[i], s[j] = s[j], s[i]
	}
}

func TestIsolatedPeerHeap(t *testing.T) {
	const numPeers = 10
	ch := testutils.NewClient(t, nil)
	defer ch.Close()

	ps1 := createSubChannelWNewStrategy(ch, "S1", numPeers, 1)
	ps2 := createSubChannelWNewStrategy(ch, "S2", numPeers, -1, Isolated)

	hostports := make([]string, numPeers)
	for i := 0; i < numPeers; i++ {
		hostports[i] = fmt.Sprintf("127.0.0.1:%d", i)
		ps1.Add(hostports[i])
		ps2.Add(hostports[i])
	}

	ps1Expected := append([]string(nil), hostports...)
	assert.Equal(t, ps1Expected, getAllPeers(t, ps1), "Unexpected peer order")

	ps2Expected := append([]string(nil), hostports...)
	reverse(ps2Expected)
	assert.Equal(t, ps2Expected, getAllPeers(t, ps2), "Unexpected peer order")
}

func TestPeerSelectionRanking(t *testing.T) {
	const numPeers = 10
	const numIterations = 1000

	// Selected is a map from rank -> [peer, count]
	// It tracks how often a peer gets selected at a specific rank.
	selected := make([]map[string]int, numPeers)
	for i := 0; i < numPeers; i++ {
		selected[i] = make(map[string]int)
	}

	for i := 0; i < numIterations; i++ {
		ch := testutils.NewClient(t, nil)
		defer ch.Close()
		ch.SetRandomSeed(int64(i * 100))

		for i := 0; i < numPeers; i++ {
			hp := fmt.Sprintf("127.0.0.1:60%v", i)
			ch.Peers().Add(hp)
		}

		for i := 0; i < numPeers; i++ {
			peer, err := ch.Peers().Get(nil)
			require.NoError(t, err, "Peers.Get failed")
			selected[i][peer.HostPort()]++
		}
	}

	for _, m := range selected {
		testDistribution(t, m, 50, 150)
	}
}

func createScoreStrategy(initial, delta int64) (calc ScoreCalculator, retCount *atomic.Uint64) {
	var (
		count atomic.Uint64
		score atomic.Uint64
	)

	return ScoreCalculatorFunc(func(p *Peer) uint64 {
		count.Add(1)
		return score.Add(uint64(delta))
	}), &count
}

func createSubChannelWNewStrategy(ch *Channel, name string, initial, delta int64, opts ...SubChannelOption) *PeerList {
	strategy, _ := createScoreStrategy(initial, delta)
	sc := ch.GetSubChannel(name, opts...)
	ps := sc.Peers()
	ps.SetStrategy(strategy)
	return ps
}

func testDistribution(t testing.TB, counts map[string]int, min, max float64) {
	for k, v := range counts {
		if float64(v) < min || float64(v) > max {
			t.Errorf("Key %v has value %v which is out of range %v-%v", k, v, min, max)
		}
	}
}

// waitTillNConnetions will run f which should end up causing the peer with hostPort in ch
// to have the specified number of inbound and outbound connections.
// If the number of connections does not match after a second, the test is failed.
func waitTillNConnections(t *testing.T, ch *Channel, hostPort string, inbound, outbound int, f func()) {
	peer, ok := ch.RootPeers().Get(hostPort)
	if !ok {
		return
	}

	var (
		i = -1
		o = -1
	)

	inboundEmpty := make(chan struct{})
	var onUpdateOnce sync.Once
	onUpdate := func(p *Peer) {
		if i, o = p.NumConnections(); (i == inbound || inbound == -1) &&
			(o == outbound || outbound == -1) {
			onUpdateOnce.Do(func() {
				close(inboundEmpty)
			})
		}
	}
	peer.SetOnUpdate(onUpdate)

	f()

	select {
	case <-inboundEmpty:
		return
	case <-time.After(time.Second):
		t.Errorf("Timed out waiting for peer %v to have (in: %v, out: %v) connections, got (in: %v, out: %v)",
			hostPort, inbound, outbound, i, o)
	}
}

// waitTillInboundEmpty will run f which should end up causing the peer with hostPort in ch
// to have 0 inbound connections. It will fail the test after a second.
func waitTillInboundEmpty(t *testing.T, ch *Channel, hostPort string, f func()) {
	waitTillNConnections(t, ch, hostPort, 0, -1, f)
}

type peerSelectionTest struct {
	peerTest

	// numPeers is the number of peers added to the client channel.
	numPeers int
	// numAffinity is the number of affinity nodes.
	numAffinity int
	// numAffinityWithNoCall is the number of affinity nodes which doesn't send call req to client.
	numAffinityWithNoCall int
	// numConcurrent is the number of concurrent goroutine to make outbound calls.
	numConcurrent int
	// hasInboundCall is the bool flag to tell whether to have inbound calls from affinity nodes
	hasInboundCall bool

	servers            []*Channel
	affinity           []*Channel
	affinityWithNoCall []*Channel
	client             *Channel
}

func (pt *peerSelectionTest) setup(t testing.TB) {
	pt.setupServers(t)
	pt.setupClient(t)
	pt.setupAffinity(t)
}

// setupServers will create numPeer servers, and register handlers on them.
func (pt *peerSelectionTest) setupServers(t testing.TB) {
	pt.servers = make([]*Channel, pt.numPeers)

	// Set up numPeers servers.
	for i := 0; i < pt.numPeers; i++ {
		pt.servers[i], _ = pt.NewService(t, "server", fmt.Sprintf("server-%v", i))
		pt.servers[i].Register(raw.Wrap(newTestHandler(pt.t)), "echo")
	}
}

func (pt *peerSelectionTest) setupAffinity(t testing.TB) {
	pt.affinity = make([]*Channel, pt.numAffinity)
	for i := range pt.affinity {
		pt.affinity[i] = pt.servers[i]
	}

	pt.affinityWithNoCall = make([]*Channel, pt.numAffinityWithNoCall)
	for i := range pt.affinityWithNoCall {
		pt.affinityWithNoCall[i] = pt.servers[i+pt.numAffinity]
	}

	var wg sync.WaitGroup
	wg.Add(pt.numAffinity)
	// Connect from the affinity nodes to the service.
	hostport := pt.client.PeerInfo().HostPort
	serviceName := pt.client.PeerInfo().ServiceName
	for _, affinity := range pt.affinity {
		go func(affinity *Channel) {
			affinity.Peers().Add(hostport)
			pt.makeCall(affinity.GetSubChannel(serviceName))
			wg.Done()
		}(affinity)
	}
	wg.Wait()

	wg.Add(pt.numAffinityWithNoCall)
	for _, p := range pt.affinityWithNoCall {
		go func(p *Channel) {
			// use ping to build connection without sending call req.
			pt.sendPing(p, hostport)
			wg.Done()
		}(p)
	}
	wg.Wait()
}

func (pt *peerSelectionTest) setupClient(t testing.TB) {
	pt.client, _ = pt.NewService(t, "client", "client")
	pt.client.Register(raw.Wrap(newTestHandler(pt.t)), "echo")
	for _, server := range pt.servers {
		pt.client.Peers().Add(server.PeerInfo().HostPort)
	}
}

func (pt *peerSelectionTest) makeCall(sc *SubChannel) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()
	_, _, _, err := raw.CallSC(ctx, sc, "echo", nil, nil)
	assert.NoError(pt.t, err, "raw.Call failed")
}

func (pt *peerSelectionTest) sendPing(ch *Channel, hostport string) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()
	err := ch.Ping(ctx, hostport)
	assert.NoError(pt.t, err, "ping failed")
}

func (pt *peerSelectionTest) runStressSimple(b *testing.B) {
	var wg sync.WaitGroup
	wg.Add(pt.numConcurrent)

	// server outbound request
	sc := pt.client.GetSubChannel("server")
	for i := 0; i < pt.numConcurrent; i++ {
		go func(sc *SubChannel) {
			defer wg.Done()
			for j := 0; j < b.N; j++ {
				pt.makeCall(sc)
			}
		}(sc)
	}

	wg.Wait()
}

func (pt *peerSelectionTest) runStress() {
	numClock := pt.numConcurrent + pt.numAffinity
	clocks := make([]chan struct{}, numClock)
	for i := 0; i < numClock; i++ {
		clocks[i] = make(chan struct{})
	}

	var wg sync.WaitGroup
	wg.Add(numClock)

	// helper that will make a request every n ticks.
	reqEveryNTicks := func(n int, sc *SubChannel, clock <-chan struct{}) {
		defer wg.Done()
		for {
			for i := 0; i < n; i++ {
				_, ok := <-clock
				if !ok {
					return
				}
			}
			pt.makeCall(sc)
		}
	}

	// server outbound request
	sc := pt.client.GetSubChannel("server")
	for i := 0; i < pt.numConcurrent; i++ {
		go reqEveryNTicks(1, sc, clocks[i])
	}
	// affinity incoming requests
	if pt.hasInboundCall {
		serviceName := pt.client.PeerInfo().ServiceName
		for i, affinity := range pt.affinity {
			go reqEveryNTicks(1, affinity.GetSubChannel(serviceName), clocks[i+pt.numConcurrent])
		}
	}

	tickAllClocks := func() {
		for i := 0; i < numClock; i++ {
			clocks[i] <- struct{}{}
		}
	}

	const tickNum = 10000
	for i := 0; i < tickNum; i++ {
		if i%(tickNum/10) == 0 {
			fmt.Printf("Stress test progress: %v\n", 100*i/tickNum)
		}
		tickAllClocks()
	}

	for i := 0; i < numClock; i++ {
		close(clocks[i])
	}
	wg.Wait()
}

// Run these commands before run the benchmark.
// sudo sysctl w kern.maxfiles=50000
// ulimit n 50000
func BenchmarkSimplePeerHeapPerf(b *testing.B) {
	pt := &peerSelectionTest{
		peerTest:      peerTest{t: b},
		numPeers:      1000,
		numConcurrent: 100,
	}
	defer pt.CleanUp()
	pt.setup(b)
	b.ResetTimer()
	pt.runStressSimple(b)
}

func TestPeerHeapPerf(t *testing.T) {
	CheckStress(t)

	tests := []struct {
		numserver      int
		affinityRatio  float64
		numConcurrent  int
		hasInboundCall bool
	}{
		{
			numserver:      1000,
			affinityRatio:  0.1,
			numConcurrent:  5,
			hasInboundCall: true,
		},
		{
			numserver:      1000,
			affinityRatio:  0.1,
			numConcurrent:  1,
			hasInboundCall: true,
		},
		{
			numserver:      100,
			affinityRatio:  0.1,
			numConcurrent:  1,
			hasInboundCall: true,
		},
	}

	for _, tt := range tests {
		peerHeapStress(t, tt.numserver, tt.affinityRatio, tt.numConcurrent, tt.hasInboundCall)
	}
}

func peerHeapStress(t testing.TB, numserver int, affinityRatio float64, numConcurrent int, hasInboundCall bool) {
	pt := &peerSelectionTest{
		peerTest:              peerTest{t: t},
		numPeers:              numserver,
		numConcurrent:         numConcurrent,
		hasInboundCall:        hasInboundCall,
		numAffinity:           int(float64(numserver) * affinityRatio),
		numAffinityWithNoCall: 3,
	}
	defer pt.CleanUp()
	pt.setup(t)
	pt.runStress()
	validateStressTest(t, pt.client, pt.numAffinity, pt.numAffinityWithNoCall)
}

func validateStressTest(t testing.TB, server *Channel, numAffinity int, numAffinityWithNoCall int) {
	state := server.IntrospectState(&IntrospectionOptions{IncludeEmptyPeers: true})

	countsByPeer := make(map[string]int)
	var counts []int
	for _, peer := range state.Peers {
		p, ok := state.RootPeers[peer.HostPort]
		assert.True(t, ok, "Missing peer.")
		if p.ChosenCount != 0 {
			countsByPeer[p.HostPort] = int(p.ChosenCount)
			counts = append(counts, int(p.ChosenCount))
		}
	}
	// when number of affinity is zero, all peer suppose to be chosen.
	if numAffinity == 0 && numAffinityWithNoCall == 0 {
		numAffinity = len(state.Peers)
	}
	assert.EqualValues(t, len(countsByPeer), numAffinity+numAffinityWithNoCall, "Number of affinities nodes mismatch.")
	sort.Ints(counts)
	median := counts[len(counts)/2]
	testDistribution(t, countsByPeer, float64(median)*0.9, float64(median)*1.1)
}

func TestPeerSelectionAfterClosed(t *testing.T) {
	pt := &peerSelectionTest{
		peerTest:    peerTest{t: t},
		numPeers:    5,
		numAffinity: 5,
	}
	defer pt.CleanUp()
	pt.setup(t)

	toClose := pt.affinity[pt.numAffinity-1]
	closedHP := toClose.PeerInfo().HostPort
	toClose.Logger().Debugf("About to Close %v", closedHP)
	waitTillInboundEmpty(t, pt.client, closedHP, func() {
		toClose.Close()
	})

	for i := 0; i < 10*pt.numAffinity; i++ {
		peer, err := pt.client.Peers().Get(nil)
		assert.NoError(t, err, "Client failed to select a peer")
		assert.NotEqual(pt.t, closedHP, peer.HostPort(), "Closed peer shouldn't be chosen")
	}
}

func TestPeerScoreOnNewConnection(t *testing.T) {
	tests := []struct {
		message string
		connect func(s1, s2 *Channel) *Peer
	}{
		{
			message: "outbound connection",
			connect: func(s1, s2 *Channel) *Peer {
				return s1.Peers().GetOrAdd(s2.PeerInfo().HostPort)
			},
		},
		{
			message: "inbound connection",
			connect: func(s1, s2 *Channel) *Peer {
				return s2.Peers().GetOrAdd(s1.PeerInfo().HostPort)
			},
		},
	}

	getScore := func(pl *PeerList) uint64 {
		peers := pl.IntrospectList(nil)
		require.Equal(t, 1, len(peers), "Wrong number of peers")
		return peers[0].Score
	}

	for _, tt := range tests {
		testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
			ctx, cancel := NewContext(time.Second)
			defer cancel()

			s1 := ts.Server()
			s2 := ts.NewServer(nil)

			s1.Peers().Add(s2.PeerInfo().HostPort)
			s2.Peers().Add(s1.PeerInfo().HostPort)

			initialScore := getScore(s1.Peers())
			peer := tt.connect(s1, s2)
			conn, err := peer.GetConnection(ctx)
			require.NoError(t, err, "%v: GetConnection failed", tt.message)

			// When receiving an inbound connection, the outbound connect may return
			// before the inbound has updated the score, so we may need to retry.
			assert.True(t, testutils.WaitFor(time.Second, func() bool {
				connectedScore := getScore(s1.Peers())
				return connectedScore < initialScore
			}), "%v: Expected connected peer score %v to be less than initial score %v",
				tt.message, getScore(s1.Peers()), initialScore)

			// Ping to ensure the connection has been added to peers on both sides.
			require.NoError(t, conn.Ping(ctx), "%v: Ping failed", tt.message)
		})
	}
}

func TestConnectToPeerHostPortMismatch(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		ctx, cancel := NewContext(time.Second)
		defer cancel()

		// Set up a relay which will have a different host:port than the
		// real TChannel HostPort.
		relay, err := benchmark.NewTCPRawRelay([]string{ts.HostPort()})
		require.NoError(t, err, "Failed to set up TCP relay")
		defer relay.Close()

		s2 := ts.NewServer(nil)
		for i := 0; i < 10; i++ {
			require.NoError(t, s2.Ping(ctx, relay.HostPort()), "Ping failed")
		}

		assert.Equal(t, 1, s2.IntrospectNumConnections(), "Unexpected number of connections")
	})
}

// Test ensures that a closing connection does not count in NumConnections.
// NumConnections should only include connections that be used to make calls.
func TestPeerConnectionsClosing(t *testing.T) {
	// Disable the relay since we check the host:port directly.
	opts := testutils.NewOpts().NoRelay()
	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		unblock := make(chan struct{})
		gotCall := make(chan struct{})
		testutils.RegisterEcho(ts.Server(), func() {
			close(gotCall)
			<-unblock
		})

		client := ts.NewServer(nil)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			testutils.AssertEcho(t, client, ts.HostPort(), ts.ServiceName())
		}()

		// Wait for the call to be received before checking connections..
		<-gotCall
		peer := ts.Server().Peers().GetOrAdd(client.PeerInfo().HostPort)
		in, out := peer.NumConnections()
		assert.Equal(t, 1, in+out, "Unexpected number of incoming connections")

		// Now when we try to close the channel, all the connections will change
		// state, and should no longer count as active connections.
		conn, err := peer.GetConnection(nil)
		require.NoError(t, err, "Failed to get connection")
		require.True(t, conn.IsActive(), "Connection should be active")

		ts.Server().Close()
		require.False(t, conn.IsActive(), "Connection should not be active after Close")
		in, out = peer.NumConnections()
		assert.Equal(t, 0, in+out, "Inactive connections should not be included in peer LAST")

		close(unblock)
		wg.Wait()
	})
}

func BenchmarkAddPeers(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ch := testutils.NewClient(b, nil)
		for i := 0; i < 1000; i++ {
			hp := fmt.Sprintf("127.0.0.1:%v", i)
			ch.Peers().Add(hp)
		}
	}
}

func TestPeerSelectionStrategyChange(t *testing.T) {
	const numPeers = 2

	ch := testutils.NewClient(t, nil)
	defer ch.Close()

	for i := 0; i < numPeers; i++ {
		ch.Peers().Add(fmt.Sprintf("127.0.0.1:60%v", i))
	}

	for _, score := range []uint64{1000, 2000} {
		ch.Peers().SetStrategy(createConstScoreStrategy(score))
		for _, v := range ch.Peers().IntrospectList(nil) {
			assert.Equal(t, v.Score, score)
		}
	}
}

func createConstScoreStrategy(score uint64) (calc ScoreCalculator) {
	return ScoreCalculatorFunc(func(p *Peer) uint64 {
		return score
	})
}
