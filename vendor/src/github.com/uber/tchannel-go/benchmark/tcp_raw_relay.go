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

package benchmark

import (
	"io"
	"log"
	"net"

	"github.com/uber-go/atomic"
)

type tcpRelay struct {
	destI      atomic.Int32
	dests      []string
	ln         net.Listener
	handleConn func(fromClient bool, src, dst net.Conn)
}

func newTCPRelay(dests []string, handleConn func(fromClient bool, src, dst net.Conn)) (*tcpRelay, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	relay := &tcpRelay{
		dests:      dests,
		ln:         ln,
		handleConn: handleConn,
	}
	go relay.acceptLoop()
	return relay, nil
}

// NewTCPRawRelay creates a relay that just pipes data from one connection
// to another directly.
func NewTCPRawRelay(dests []string) (Relay, error) {
	return newTCPRelay(dests, func(_ bool, src, dst net.Conn) {
		io.Copy(src, dst)
	})
}

func (r *tcpRelay) acceptLoop() {
	for {
		conn, err := r.ln.Accept()
		if err, ok := err.(net.Error); ok && err.Temporary() {
			continue
		}
		if err != nil {
			return
		}

		go r.handleIncoming(conn)
	}
}

func (r *tcpRelay) handleIncoming(src net.Conn) {
	defer src.Close()

	dst, err := net.Dial("tcp", r.nextDestination())
	if err != nil {
		log.Printf("Connection failed: %v", err)
		return
	}
	defer dst.Close()

	go r.handleConn(true, src, dst)
	r.handleConn(false, dst, src)
}

func (r *tcpRelay) nextDestination() string {
	i := int(r.destI.Inc()-1) % len(r.dests)
	return r.dests[i]
}

func (r *tcpRelay) HostPort() string {
	return r.ln.Addr().String()
}

func (r *tcpRelay) Close() {
	r.ln.Close()
}
