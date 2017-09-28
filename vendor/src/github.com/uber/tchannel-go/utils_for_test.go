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

package tchannel

// This file contains functions for tests to access internal tchannel state.
// Since it has a _test.go suffix, it is only compiled with tests in this package.

import (
	"net"
	"time"

	"golang.org/x/net/context"
)

// MexChannelBufferSize is the size of the message exchange channel buffer.
const MexChannelBufferSize = mexChannelBufferSize

// SetOnUpdate sets onUpdate for a peer, which is called when the peer's score is
// updated in all peer lists.
func (p *Peer) SetOnUpdate(f func(*Peer)) {
	p.Lock()
	p.onUpdate = f
	p.Unlock()
}

// GetConnectionRelay exports the getConnectionRelay for tests.
func (p *Peer) GetConnectionRelay(timeout time.Duration) (*Connection, error) {
	return p.getConnectionRelay(timeout)
}

// SetRandomSeed seeds all the random number generators in the channel so that
// tests will be deterministic for a given seed.
func (ch *Channel) SetRandomSeed(seed int64) {
	ch.Peers().peerHeap.rng.Seed(seed)
	peerRng.Seed(seed)
	for _, sc := range ch.subChannels.subchannels {
		sc.peers.peerHeap.rng.Seed(seed + int64(len(sc.peers.peersByHostPort)))
	}
}

// Ping sends a ping on the specific connection.
func (c *Connection) Ping(ctx context.Context) error {
	return c.ping(ctx)
}

// Logger returns the logger for the specific connection.
func (c *Connection) Logger() Logger {
	return c.log
}

// OutboundConnection returns the underlying connection for an outbound call.
func OutboundConnection(call *OutboundCall) (*Connection, net.Conn) {
	conn := call.conn
	return conn, conn.conn
}

// InboundConnection returns the underlying connection for an incoming call.
func InboundConnection(call IncomingCall) (*Connection, net.Conn) {
	inboundCall, ok := call.(*InboundCall)
	if !ok {
		return nil, nil
	}

	conn := inboundCall.conn
	return conn, conn.conn
}
