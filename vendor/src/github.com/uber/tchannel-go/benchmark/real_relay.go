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
	"errors"
	"os"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/relay"
	"github.com/uber/tchannel-go/relay/relaytest"

	"github.com/uber-go/atomic"
)

type fixedHosts struct {
	hosts map[string][]string
	pickI atomic.Int32
}

func (fh *fixedHosts) Get(cf relay.CallFrame, _ *tchannel.Connection) (string, error) {
	peers := fh.hosts[string(cf.Service())]
	if len(peers) == 0 {
		return "", errors.New("no peers")
	}

	pickI := int(fh.pickI.Inc()-1) % len(peers)
	return peers[pickI], nil
}

type realRelay struct {
	ch    *tchannel.Channel
	hosts *fixedHosts
}

// NewRealRelay creates a TChannel relay.
func NewRealRelay(services map[string][]string) (Relay, error) {
	hosts := &fixedHosts{hosts: services}
	ch, err := tchannel.NewChannel("relay", &tchannel.ChannelOptions{
		RelayHost: relaytest.HostFunc(hosts.Get),
		Logger:    tchannel.NewLevelLogger(tchannel.NewLogger(os.Stderr), tchannel.LogLevelWarn),
	})
	if err != nil {
		return nil, err
	}

	if err := ch.ListenAndServe("127.0.0.1:0"); err != nil {
		return nil, err
	}

	return &realRelay{
		ch:    ch,
		hosts: hosts,
	}, nil
}

func (r *realRelay) HostPort() string {
	return r.ch.PeerInfo().HostPort
}

func (r *realRelay) Close() {
	r.ch.Close()
}
