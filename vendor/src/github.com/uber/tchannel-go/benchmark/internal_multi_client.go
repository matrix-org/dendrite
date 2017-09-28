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
	"time"

	"github.com/uber/tchannel-go/testutils"
)

type internalMultiClient struct {
	clients []inProcClient
}

func newInternalMultiClient(hosts []string, opts *options) Client {
	clients := make([]inProcClient, opts.numClients)
	opts.numClients = 1

	for i := range clients {
		clients[i] = newClient(hosts, opts)
	}

	return &internalMultiClient{clients: clients}
}

func (c *internalMultiClient) Warmup() error {
	for _, c := range c.clients {
		if err := c.Warmup(); err != nil {
			return err
		}
	}
	return nil
}

func (c *internalMultiClient) Close() {
	for _, client := range c.clients {
		client.Close()
	}
}

func (c *internalMultiClient) RawCall(n int) ([]time.Duration, error) {
	return c.makeCalls(n, func(c inProcClient) callFunc {
		return c.RawCallBuffer
	})
}

func (c *internalMultiClient) ThriftCall(n int) ([]time.Duration, error) {
	return c.makeCalls(n, func(c inProcClient) callFunc {
		return c.ThriftCallBuffer
	})
}

type callFunc func([]time.Duration) error

type clientToCallFunc func(c inProcClient) callFunc

func (c *internalMultiClient) makeCalls(n int, f clientToCallFunc) ([]time.Duration, error) {
	buckets := testutils.Buckets(n, len(c.clients))
	errCs := make([]chan error, len(c.clients))

	var start int
	latencies := make([]time.Duration, n)
	for i := range c.clients {
		calls := buckets[i]

		end := start + calls
		errCs[i] = c.callUsingClient(latencies[start:end], f(c.clients[i]))
		start = end
	}

	for _, errC := range errCs {
		if err := <-errC; err != nil {
			return nil, err
		}
	}

	return latencies, nil
}

func (c *internalMultiClient) callUsingClient(latencies []time.Duration, f callFunc) chan error {
	errC := make(chan error, 1)
	if len(latencies) == 0 {
		errC <- nil
		return errC
	}

	go func() {
		errC <- f(latencies)
	}()
	return errC
}
