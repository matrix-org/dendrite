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
)
import "github.com/uber/tchannel-go"

type tcpFrameRelay struct {
	*tcpRelay
	modifier func(bool, *tchannel.Frame) *tchannel.Frame
}

// NewTCPFrameRelay relays frames from one connection to another. It reads
// and writes frames using the TChannel frame functions.
func NewTCPFrameRelay(dests []string, modifier func(bool, *tchannel.Frame) *tchannel.Frame) (Relay, error) {
	var err error
	r := &tcpFrameRelay{modifier: modifier}

	r.tcpRelay, err = newTCPRelay(dests, r.handleConnFrameRelay)
	if err != nil {
		return nil, err
	}

	return r, nil
}

func (r *tcpFrameRelay) handleConnFrameRelay(fromClient bool, src, dst net.Conn) {
	pool := tchannel.NewSyncFramePool()
	frameCh := make(chan *tchannel.Frame, 100)
	defer close(frameCh)

	go func() {
		for f := range frameCh {
			if err := f.WriteOut(dst); err != nil {
				log.Printf("Failed to write out frame: %v", err)
				return
			}
			pool.Release(f)
		}
	}()

	for {
		f := pool.Get()
		if err := f.ReadIn(src); err != nil {
			return
		}

		if r.modifier != nil {
			f = r.modifier(fromClient, f)
		}

		select {
		case frameCh <- f:
		default:
			panic("frame buffer full")
		}
	}
}
