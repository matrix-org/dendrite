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

package thrift_test

import (
	"flag"
	"testing"
	"time"

	"github.com/uber/tchannel-go/benchmark"
	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/require"
	"github.com/uber-go/atomic"
)

const callBatch = 100

var (
	useHyperbahn   = flag.Bool("useHyperbahn", false, "Whether to advertise and route requests through Hyperbahn")
	hyperbahnNodes = flag.String("hyperbahn-nodes", "127.0.0.1:21300,127.0.0.1:21301", "Comma-separated list of Hyperbahn nodes")
	requestSize    = flag.Int("request-size", 4, "Call payload size")
	timeout        = flag.Duration("call-timeout", time.Second, "Timeout for each call")
)

func init() {
	benchmark.BenchmarkDir = "../benchmark/"
}

func BenchmarkBothSerial(b *testing.B) {
	server := benchmark.NewServer()
	client := benchmark.NewClient(
		[]string{server.HostPort()},
		benchmark.WithTimeout(*timeout),
		benchmark.WithRequestSize(*requestSize),
	)

	b.ResetTimer()

	for _, calls := range testutils.Batch(b.N, callBatch) {
		if _, err := client.ThriftCall(calls); err != nil {
			b.Errorf("Call failed: %v", err)
		}
	}
}

func BenchmarkInboundSerial(b *testing.B) {
	server := benchmark.NewServer()
	client := benchmark.NewClient(
		[]string{server.HostPort()},
		benchmark.WithTimeout(*timeout),
		benchmark.WithExternalProcess(),
		benchmark.WithRequestSize(*requestSize),
	)
	defer client.Close()
	require.NoError(b, client.Warmup(), "Warmup failed")

	b.ResetTimer()
	for _, calls := range testutils.Batch(b.N, callBatch) {
		if _, err := client.ThriftCall(calls); err != nil {
			b.Errorf("Call failed: %v", err)
		}
	}
}

func BenchmarkInboundParallel(b *testing.B) {
	server := benchmark.NewServer()

	var reqCounter atomic.Int32
	started := time.Now()

	b.RunParallel(func(pb *testing.PB) {
		client := benchmark.NewClient(
			[]string{server.HostPort()},
			benchmark.WithTimeout(*timeout),
			benchmark.WithExternalProcess(),
			benchmark.WithRequestSize(*requestSize),
		)
		defer client.Close()
		require.NoError(b, client.Warmup(), "Warmup failed")

		for pb.Next() {
			if _, err := client.ThriftCall(100); err != nil {
				b.Errorf("Call failed: %v", err)
			}
			reqCounter.Add(100)
		}
	})

	duration := time.Since(started)
	reqs := reqCounter.Load()
	b.Logf("Requests: %v   RPS: %v", reqs, float64(reqs)/duration.Seconds())
}
