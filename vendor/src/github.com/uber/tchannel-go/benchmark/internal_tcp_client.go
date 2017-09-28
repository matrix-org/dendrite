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
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/uber/tchannel-go"
)

// internalTCPClient represents a TCP client that makes
// TChannel calls using raw TCP packets.
type internalTCPClient struct {
	host        string
	lastID      uint32
	responseIDs chan uint32
	conn        net.Conn
	frames      frames
	opts        *options
}

func newInternalTCPClient(hosts []string, opts *options) inProcClient {
	return &internalTCPClient{
		host:        hosts[rand.Intn(len(hosts))],
		responseIDs: make(chan uint32, 1000),
		frames:      getRawCallFrames(opts.timeout, opts.svcName, opts.reqSize),
		lastID:      1,
		opts:        opts,
	}
}

func (c *internalTCPClient) Warmup() error {
	conn, err := net.Dial("tcp", c.host)
	if err != nil {
		return err
	}

	c.conn = conn
	go c.readConn()

	if err := c.frames.writeInitReq(conn); err != nil {
		panic(err)
	}

	return nil
}

func (c *internalTCPClient) readConn() {
	defer close(c.responseIDs)

	wantFirstID := true
	f := tchannel.NewFrame(tchannel.MaxFrameSize)
	for {
		err := f.ReadIn(c.conn)
		if err != nil {
			return
		}

		if wantFirstID {
			if f.Header.ID != 1 {
				panic(fmt.Errorf("Expected first response ID to be 1, got %v", f.Header.ID))
			}
			wantFirstID = false
			continue
		}

		c.responseIDs <- f.Header.ID
	}
}

type call struct {
	id        uint32
	started   time.Time
	numFrames int
}

func (c *internalTCPClient) makeCalls(latencies []time.Duration, f func() (call, error)) error {
	n := len(latencies)
	calls := make(map[uint32]*call, n)

	for i := 0; i < n; i++ {
		c, err := f()
		if err != nil {
			return err
		}

		calls[c.id] = &c
	}

	timer := time.NewTimer(c.opts.timeout)

	// Use the original underlying slice for latencies.
	durations := latencies[:0]
	for {
		if len(calls) == 0 {
			return nil
		}

		timer.Reset(c.opts.timeout)
		select {
		case id, ok := <-c.responseIDs:
			if !ok {
				panic("expecting more calls, but connection is closed")
			}
			call, ok := calls[id]
			if !ok {
				panic(fmt.Errorf("received unexpected response frame: %v", id))
			}

			call.numFrames--
			if call.numFrames != 0 {
				continue
			}
			durations = append(durations, time.Since(call.started))
			delete(calls, id)
		case <-timer.C:
			return tchannel.ErrTimeout
		}
	}
}

func (c *internalTCPClient) RawCallBuffer(latencies []time.Duration) error {
	return c.makeCalls(latencies, func() (call, error) {
		c.lastID++

		started := time.Now()
		numFrames, err := c.frames.writeCallReq(c.lastID, c.conn)
		if err != nil {
			return call{}, err
		}

		return call{c.lastID, started, numFrames}, nil
	})
}

func (c *internalTCPClient) RawCall(n int) ([]time.Duration, error) {
	latencies := make([]time.Duration, n)
	return latencies, c.RawCallBuffer(latencies)
}

func (c *internalTCPClient) ThriftCallBuffer(latencies []time.Duration) error {
	panic("not yet implemented")
}

func (c *internalTCPClient) ThriftCall(n int) ([]time.Duration, error) {
	panic("not yet implemented")
}

func (c *internalTCPClient) Close() {
	c.conn.Close()
}
