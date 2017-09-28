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
	"log"
	"net"

	"github.com/uber/tchannel-go"

	"github.com/uber-go/atomic"
)

// internalTCPServer represents a TCP server responds to TChannel
// calls using raw TCP packets.
type internalTCPServer struct {
	frames   frames
	ln       net.Listener
	opts     *options
	rawCalls atomic.Int64
}

func newInternalTCPServer(opts *options) Server {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	s := &internalTCPServer{
		ln:     ln,
		frames: getRawCallFrames(opts.timeout, opts.svcName, opts.reqSize),
		opts:   opts,
	}
	go s.acceptLoop()
	return s
}

func (s *internalTCPServer) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err, ok := err.(net.Error); ok && err.Temporary() {
			continue
		}
		if err != nil {
			return
		}

		go s.handleConn(conn)
	}
}

func (s *internalTCPServer) handleConn(conn net.Conn) {
	c := make(chan uint32, 1000)
	defer close(c)
	go s.writeResponses(conn, c)

	var lastID uint32

	f := tchannel.NewFrame(tchannel.MaxFrameSize)
	for {
		if err := f.ReadIn(conn); err != nil {
			return
		}

		if f.Header.ID > lastID {
			c <- f.Header.ID
			lastID = f.Header.ID
		}
	}
}

func (s *internalTCPServer) writeResponses(conn net.Conn, ids chan uint32) {
	frames := s.frames.duplicate()

	for id := range ids {
		if id == 1 {
			if err := frames.writeInitRes(conn); err != nil {
				log.Printf("writeInitRes failed: %v", err)
			}
			continue
		}

		s.rawCalls.Inc()
		if _, err := frames.writeCallRes(id, conn); err != nil {
			log.Printf("writeCallRes failed: %v", err)
			return
		}
	}
}

func (s *internalTCPServer) HostPort() string {
	return s.ln.Addr().String()
}

func (s *internalTCPServer) RawCalls() int {
	return int(s.rawCalls.Load())
}

func (s *internalTCPServer) ThriftCalls() int {
	// Server does not support Thrift calls currently.
	return 0
}

func (s *internalTCPServer) Close() {
	s.ln.Close()
}
