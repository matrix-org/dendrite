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
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/thrift"
	gen "github.com/uber/tchannel-go/thrift/gen-go/test"
)

// internalClient represents a benchmark client.
type internalClient struct {
	ch          *tchannel.Channel
	sc          *tchannel.SubChannel
	tClient     gen.TChanSecondService
	argStr      string
	argBytes    []byte
	checkResult bool
	opts        *options
}

// NewClient returns a new Client that can make calls to a benchmark server.
func NewClient(hosts []string, optFns ...Option) Client {
	opts := getOptions(optFns)
	if opts.external {
		return newExternalClient(hosts, opts)
	}
	if opts.numClients > 1 {
		return newInternalMultiClient(hosts, opts)
	}
	return newClient(hosts, opts)
}

func newClient(hosts []string, opts *options) inProcClient {
	if opts.external || opts.numClients > 1 {
		panic("newClient got options that should be handled by NewClient")
	}

	if opts.noLibrary {
		return newInternalTCPClient(hosts, opts)
	}
	return newInternalClient(hosts, opts)
}

func newInternalClient(hosts []string, opts *options) inProcClient {
	ch, err := tchannel.NewChannel(opts.svcName, &tchannel.ChannelOptions{
		Logger: tchannel.NewLevelLogger(tchannel.NewLogger(os.Stderr), tchannel.LogLevelWarn),
	})
	if err != nil {
		panic("failed to create channel: " + err.Error())
	}
	for _, host := range hosts {
		ch.Peers().Add(host)
	}
	thriftClient := thrift.NewClient(ch, opts.svcName, nil)
	client := gen.NewTChanSecondServiceClient(thriftClient)

	return &internalClient{
		ch:       ch,
		sc:       ch.GetSubChannel(opts.svcName),
		tClient:  client,
		argBytes: getRequestBytes(opts.reqSize),
		argStr:   getRequestString(opts.reqSize),
		opts:     opts,
	}
}

func (c *internalClient) Warmup() error {
	for _, peer := range c.ch.Peers().Copy() {
		ctx, cancel := tchannel.NewContext(c.opts.timeout)
		_, err := peer.GetConnection(ctx)
		cancel()

		if err != nil {
			return err
		}
	}

	return nil
}

func (c *internalClient) makeCalls(latencies []time.Duration, f func() (time.Duration, error)) error {
	for i := range latencies {
		var err error
		latencies[i], err = f()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *internalClient) RawCallBuffer(latencies []time.Duration) error {
	return c.makeCalls(latencies, func() (time.Duration, error) {
		ctx, cancel := tchannel.NewContext(c.opts.timeout)
		defer cancel()

		started := time.Now()
		rArg2, rArg3, _, err := raw.CallSC(ctx, c.sc, "echo", c.argBytes, c.argBytes)
		duration := time.Since(started)

		if err != nil {
			return 0, err
		}
		if c.checkResult {
			if !bytes.Equal(rArg2, c.argBytes) || !bytes.Equal(rArg3, c.argBytes) {
				fmt.Println("Arg2", rArg2, "Expect", c.argBytes)
				fmt.Println("Arg3", rArg3, "Expect", c.argBytes)
				panic("echo call returned wrong results")
			}
		}
		return duration, nil
	})
}

func (c *internalClient) RawCall(n int) ([]time.Duration, error) {
	latencies := make([]time.Duration, n)
	return latencies, c.RawCallBuffer(latencies)
}

func (c *internalClient) ThriftCallBuffer(latencies []time.Duration) error {
	return c.makeCalls(latencies, func() (time.Duration, error) {
		ctx, cancel := thrift.NewContext(c.opts.timeout)
		defer cancel()

		started := time.Now()
		res, err := c.tClient.Echo(ctx, c.argStr)
		duration := time.Since(started)

		if err != nil {
			return 0, err
		}
		if c.checkResult {
			if res != c.argStr {
				panic("thrift Echo returned wrong result")
			}
		}
		return duration, nil
	})
}

func (c *internalClient) ThriftCall(n int) ([]time.Duration, error) {
	latencies := make([]time.Duration, n)
	return latencies, c.ThriftCallBuffer(latencies)
}

func (c *internalClient) Close() {
	c.ch.Close()
}
