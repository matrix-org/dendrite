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

package testutils

import (
	"io"
	"net"
	"sync"
	"testing"

	"github.com/uber/tchannel-go"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/atomic"
)

type frameRelay struct {
	sync.Mutex // protects conns

	t           *testing.T
	destination string
	relayFunc   func(outgoing bool, f *tchannel.Frame) *tchannel.Frame
	closed      atomic.Uint32
	conns       []net.Conn
	wg          sync.WaitGroup
}

func (r *frameRelay) listen() (listenHostPort string, cancel func()) {
	conn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(r.t, err, "net.Listen failed")

	go func() {
		for {
			c, err := conn.Accept()
			if err != nil {
				if r.closed.Load() == 0 {
					r.t.Errorf("Accept failed: %v", err)
				}
				return
			}

			r.Lock()
			r.conns = append(r.conns, c)
			r.Unlock()

			r.relayConn(c)
		}
	}()

	return conn.Addr().String(), func() {
		r.closed.Inc()
		conn.Close()
		r.Lock()
		for _, c := range r.conns {
			c.Close()
		}
		r.Unlock()
		// Wait for all the outbound connections we created to close.
		r.wg.Wait()
	}
}

func (r *frameRelay) relayConn(c net.Conn) {
	outC, err := net.Dial("tcp", r.destination)
	if !assert.NoError(r.t, err, "relay connection failed") {
		return
	}
	r.Lock()
	defer r.Unlock()

	if r.closed.Load() > 0 {
		outC.Close()
		return
	}

	r.conns = append(r.conns, outC)

	r.wg.Add(2)
	go r.relayBetween(true /* outgoing */, c, outC)
	go r.relayBetween(false /* outgoing */, outC, c)
}

func (r *frameRelay) relayBetween(outgoing bool, c net.Conn, outC net.Conn) {
	defer r.wg.Done()

	frame := tchannel.NewFrame(tchannel.MaxFramePayloadSize)
	for {
		err := frame.ReadIn(c)
		if err == io.EOF {
			// Connection gracefully closed.
			return
		}
		if err != nil && r.closed.Load() > 0 {
			// Once the relay is shutdown, we expect connection errors.
			return
		}
		if !assert.NoError(r.t, err, "read frame failed") {
			return
		}

		outFrame := r.relayFunc(outgoing, frame)
		if outFrame == nil {
			continue
		}

		err = outFrame.WriteOut(outC)
		if err != nil && r.closed.Load() > 0 {
			// Once the relay is shutdown, we expect connection errors.
			return
		}
		if !assert.NoError(r.t, err, "write frame failed") {
			return
		}
	}
}

// FrameRelay sets up a relay that can modify frames using relayFunc.
func FrameRelay(t *testing.T, destination string, relayFunc func(outgoing bool, f *tchannel.Frame) *tchannel.Frame) (listenHostPort string, cancel func()) {
	relay := &frameRelay{
		t:           t,
		destination: destination,
		relayFunc:   relayFunc,
	}
	return relay.listen()
}
