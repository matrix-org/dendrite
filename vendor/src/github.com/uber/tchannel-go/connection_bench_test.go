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

package tchannel_test

import (
	"runtime"
	"sync"
	"testing"
	"time"

	. "github.com/uber/tchannel-go"

	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"

	"github.com/streadway/quantile"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

const benchService = "bench-server"

type benchmarkHandler struct{}

func (h *benchmarkHandler) Handle(ctx context.Context, args *raw.Args) (*raw.Res, error) {
	return &raw.Res{
		Arg2: args.Arg3,
		Arg3: args.Arg2,
	}, nil
}

func (h *benchmarkHandler) OnError(ctx context.Context, err error) {
}

type latencyTracker struct {
	sync.Mutex

	started   time.Time
	estimator *quantile.Estimator
}

func newLatencyTracker() *latencyTracker {
	return &latencyTracker{
		estimator: quantile.New(
			quantile.Unknown(0.01),
			quantile.Known(0.50, 0.01),
			quantile.Known(0.95, 0.001),
			quantile.Known(0.99, 0.0005),
			quantile.Known(1.0, 0.0005),
		),
		started: time.Now(),
	}
}

func (lt *latencyTracker) addLatency(d time.Duration) {
	lt.Lock()
	lt.estimator.Add(float64(d))
	lt.Unlock()
}

func (lt *latencyTracker) report(t testing.TB) {
	duration := time.Since(lt.started)
	lt.Lock()
	t.Logf("%6v calls, %5.0f RPS (%v per call). Latency: Average: %v P95: %v P99: %v P100: %v",
		lt.estimator.Samples(),
		float64(lt.estimator.Samples())/float64(duration)*float64(time.Second),
		duration/time.Duration(lt.estimator.Samples()),
		time.Duration(lt.estimator.Get(0.50)),
		time.Duration(lt.estimator.Get(0.95)),
		time.Duration(lt.estimator.Get(0.99)),
		time.Duration(lt.estimator.Get(1.0)),
	)
	lt.Unlock()
}

func setupServer(t testing.TB) *Channel {
	serverCh := testutils.NewServer(t, testutils.NewOpts().SetServiceName("bench-server"))
	handler := &benchmarkHandler{}
	serverCh.Register(raw.Wrap(handler), "echo")
	return serverCh
}

type benchmarkConfig struct {
	numCalls         int
	numServers       int
	numClients       int
	workersPerClient int
	numBytes         int
}

func benchmarkCallsN(b *testing.B, c benchmarkConfig) {
	var (
		clients []*Channel
		servers []*Channel
	)
	lt := newLatencyTracker()

	if c.numBytes == 0 {
		c.numBytes = 100
	}
	data := testutils.RandBytes(c.numBytes)

	// Set up clients and servers.
	for i := 0; i < c.numServers; i++ {
		servers = append(servers, setupServer(b))
	}
	for i := 0; i < c.numClients; i++ {
		clients = append(clients, testutils.NewClient(b, nil))
		for _, s := range servers {
			clients[i].Peers().Add(s.PeerInfo().HostPort)

			// Initialize a connection
			ctx, cancel := NewContext(50 * time.Millisecond)
			assert.NoError(b, clients[i].Ping(ctx, s.PeerInfo().HostPort), "Initial ping failed")
			cancel()
		}
	}

	// Make calls from clients to the servers
	call := func(sc *SubChannel) {
		ctx, cancel := NewContext(50 * time.Millisecond)
		start := time.Now()
		_, _, _, err := raw.CallSC(ctx, sc, "echo", nil, data)
		duration := time.Since(start)
		cancel()
		if assert.NoError(b, err, "Call failed") {
			lt.addLatency(duration)
		}
	}

	reqsLeft := testutils.Decrementor(c.numCalls)
	clientWorker := func(client *Channel, clientNum, workerNum int) {
		sc := client.GetSubChannel(benchService)
		for reqsLeft.Single() {
			call(sc)
		}
	}
	clientRunner := func(client *Channel, clientNum int) {
		testutils.RunN(c.workersPerClient, func(i int) {
			clientWorker(client, clientNum, i)
		})
	}

	lt = newLatencyTracker()
	defer lt.report(b)
	b.ResetTimer()

	testutils.RunN(c.numClients, func(i int) {
		clientRunner(clients[i], i)
	})
}

func BenchmarkCallsSerial(b *testing.B) {
	benchmarkCallsN(b, benchmarkConfig{
		numCalls:         b.N,
		numServers:       1,
		numClients:       1,
		workersPerClient: 1,
	})
}

func BenchmarkCallsConcurrentServer(b *testing.B) {
	benchmarkCallsN(b, benchmarkConfig{
		numCalls:         b.N,
		numServers:       1,
		numClients:       runtime.GOMAXPROCS(0),
		workersPerClient: 1,
	})
}

func BenchmarkCallsConcurrentClient(b *testing.B) {
	parallelism := runtime.GOMAXPROCS(0)
	benchmarkCallsN(b, benchmarkConfig{
		numCalls:         b.N,
		numServers:       parallelism,
		numClients:       1,
		workersPerClient: parallelism,
	})
}
