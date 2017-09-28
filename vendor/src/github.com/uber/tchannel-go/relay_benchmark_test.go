package tchannel_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/uber/tchannel-go"

	"github.com/uber/tchannel-go/benchmark"
	"github.com/uber/tchannel-go/testutils"

	"github.com/bmizerany/perks/quantile"
	"github.com/stretchr/testify/require"
)

type benchmarkParams struct {
	servers, clients int
	requestSize      int
}

type workerControl struct {
	start        sync.WaitGroup
	unblockStart chan struct{}
	done         sync.WaitGroup
}

func init() {
	benchmark.BenchmarkDir = "./benchmark/"
}

func newWorkerControl(numWorkers int) *workerControl {
	wc := &workerControl{
		unblockStart: make(chan struct{}),
	}
	wc.start.Add(numWorkers)
	wc.done.Add(numWorkers)
	return wc
}

func (c *workerControl) WaitForStart(f func()) {
	c.start.Wait()
	f()
	close(c.unblockStart)
}

func (c *workerControl) WaitForEnd() {
	c.done.Wait()
}

func (c *workerControl) WorkerStart() {
	c.start.Done()
	<-c.unblockStart
}

func (c *workerControl) WorkerDone() {
	c.done.Done()
}

func defaultParams() benchmarkParams {
	return benchmarkParams{
		servers:     2,
		clients:     2,
		requestSize: 1024,
	}
}

func closeAndVerify(b *testing.B, ch *Channel) {
	ch.Close()
	isChanClosed := func() bool {
		return ch.State() == ChannelClosed
	}
	if !testutils.WaitFor(time.Second, isChanClosed) {
		b.Errorf("Timed out waiting for channel to close, state: %v", ch.State())
	}
}

func benchmarkRelay(b *testing.B, p benchmarkParams) {
	b.SetBytes(int64(p.requestSize))
	b.ReportAllocs()

	services := make(map[string][]string)

	servers := make([]benchmark.Server, p.servers)
	for i := range servers {
		servers[i] = benchmark.NewServer(
			benchmark.WithServiceName("svc"),
			benchmark.WithRequestSize(p.requestSize),
			benchmark.WithExternalProcess(),
		)
		defer servers[i].Close()
		services["svc"] = append(services["svc]"], servers[i].HostPort())
	}

	relay, err := benchmark.NewRealRelay(services)
	require.NoError(b, err, "Failed to create relay")
	defer relay.Close()

	clients := make([]benchmark.Client, p.clients)
	for i := range clients {
		clients[i] = benchmark.NewClient([]string{relay.HostPort()},
			benchmark.WithServiceName("svc"),
			benchmark.WithRequestSize(p.requestSize),
			benchmark.WithExternalProcess(),
			benchmark.WithTimeout(10*time.Second),
		)
		defer clients[i].Close()
		require.NoError(b, clients[i].Warmup(), "Warmup failed")
	}

	quantileVals := []float64{0.50, 0.95, 0.99, 1.0}
	quantiles := make([]*quantile.Stream, p.clients)
	for i := range quantiles {
		quantiles[i] = quantile.NewTargeted(quantileVals...)
	}

	wc := newWorkerControl(p.clients)
	dec := testutils.Decrementor(b.N)

	for i, c := range clients {
		go func(i int, c benchmark.Client) {
			// Do a warm up call.
			c.RawCall(1)

			wc.WorkerStart()
			defer wc.WorkerDone()

			for {
				tokens := dec.Multiple(200)
				if tokens == 0 {
					break
				}

				durations, err := c.RawCall(tokens)
				if err != nil {
					b.Fatalf("Call failed: %v", err)
				}

				for _, d := range durations {
					quantiles[i].Insert(float64(d))
				}
			}
		}(i, c)
	}

	var started time.Time
	wc.WaitForStart(func() {
		b.ResetTimer()
		started = time.Now()
	})
	wc.WaitForEnd()
	duration := time.Since(started)

	fmt.Printf("\nb.N: %v Duration: %v RPS = %0.0f\n", b.N, duration, float64(b.N)/duration.Seconds())

	// Merge all the quantiles into 1
	for _, q := range quantiles[1:] {
		quantiles[0].Merge(q.Samples())
	}

	for _, q := range quantileVals {
		fmt.Printf("  %0.4f = %v\n", q, time.Duration(quantiles[0].Query(q)))
	}
	fmt.Println()
}

func BenchmarkRelayNoLatencies(b *testing.B) {
	server := benchmark.NewServer(
		benchmark.WithServiceName("svc"),
		benchmark.WithExternalProcess(),
		benchmark.WithNoLibrary(),
	)
	defer server.Close()

	hostMapping := map[string][]string{"svc": {server.HostPort()}}
	relay, err := benchmark.NewRealRelay(hostMapping)
	require.NoError(b, err, "NewRealRelay failed")
	defer relay.Close()

	client := benchmark.NewClient([]string{relay.HostPort()},
		benchmark.WithServiceName("svc"),
		benchmark.WithExternalProcess(),
		benchmark.WithNoLibrary(),
		benchmark.WithNumClients(10),
		benchmark.WithNoChecking(),
		benchmark.WithNoDurations(),
		benchmark.WithTimeout(10*time.Second),
	)
	defer client.Close()
	require.NoError(b, client.Warmup(), "client.Warmup failed")

	b.ResetTimer()
	started := time.Now()
	for _, calls := range testutils.Batch(b.N, 10000) {
		if _, err := client.RawCall(calls); err != nil {
			b.Fatalf("Calls failed: %v", err)
		}
	}

	duration := time.Since(started)
	fmt.Printf("\nb.N: %v Duration: %v RPS = %0.0f\n", b.N, duration, float64(b.N)/duration.Seconds())
}

func BenchmarkRelay2Servers5Clients1k(b *testing.B) {
	p := defaultParams()
	p.clients = 5
	p.servers = 2
	benchmarkRelay(b, p)
}

func BenchmarkRelay4Servers20Clients1k(b *testing.B) {
	p := defaultParams()
	p.clients = 20
	p.servers = 4
	benchmarkRelay(b, p)
}

func BenchmarkRelay2Servers5Clients4k(b *testing.B) {
	p := defaultParams()
	p.requestSize = 4 * 1024
	p.clients = 5
	p.servers = 2
	benchmarkRelay(b, p)
}
