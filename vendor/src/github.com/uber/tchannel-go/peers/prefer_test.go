// Copyright (c) 2017 Uber Technologies, Inc.

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

package peers

import (
	"fmt"
	"hash/fnv"
	"testing"
	"time"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/atomic"
	"golang.org/x/net/context"
)

func TestHRWScorerGetScore(t *testing.T) {
	client := testutils.NewClient(t, nil)
	peer := client.Peers().GetOrAdd("1.1.1.1")

	c1 := NewHRWScorer(1)
	c2 := NewHRWScorer(2)

	assert.NotEqual(t, c1.GetScore(peer), c2.GetScore(peer))
}

func TestHRWScorerDistribution(t *testing.T) {
	const (
		numClients = 1000
		numServers = 10
	)

	ch := testutils.NewClient(t, nil)
	servers := make([]*tchannel.Peer, numServers)
	for i := range servers {
		servers[i] = ch.Peers().GetOrAdd(fmt.Sprintf("192.0.2.%v", i))
	}

	serverSelected := make([]int, numServers)
	for i := 0; i < numClients; i++ {
		client := NewHRWScorer(uint32(i))

		highestScore := uint64(0)
		highestServer := -1
		for s, server := range servers {
			if score := client.GetScore(server); score > highestScore {
				highestScore = score
				highestServer = s
			}
		}
		serverSelected[highestServer]++
	}

	// We can't get a perfect distribution, but should be within 20%.
	const (
		expectCalls = numClients / numServers
		delta       = expectCalls * 0.2
	)
	for serverIdx, count := range serverSelected {
		assert.InDelta(t, expectCalls, count, delta, "Server %v out of range", serverIdx)
	}
}

func countingServer(t *testing.T, opts *testutils.ChannelOpts) (*tchannel.Channel, *atomic.Int32) {
	var cnt atomic.Int32
	server := testutils.NewServer(t, opts)
	testutils.RegisterEcho(server, func() { cnt.Inc() })
	return server, &cnt
}

func TestHRWScorerIntegration(t *testing.T) {
	// Client pings to the server may cause errors during Close.
	sOpts := testutils.NewOpts().SetServiceName("svc").DisableLogVerification()
	s1, s1Count := countingServer(t, sOpts)
	s2, s2Count := countingServer(t, sOpts)

	client := testutils.NewClient(t, testutils.NewOpts().DisableLogVerification())
	client.Peers().SetStrategy(NewHRWScorer(1))
	client.Peers().Add(s1.PeerInfo().HostPort)
	client.Peers().Add(s2.PeerInfo().HostPort)

	// We want to call the raw echo function with TChannel retries.
	callEcho := func() error {
		ctx, cancel := tchannel.NewContext(time.Second)
		defer cancel()
		return client.RunWithRetry(ctx, func(ctx context.Context, rs *tchannel.RequestState) error {
			_, err := raw.CallV2(ctx, client.GetSubChannel("svc"), raw.CArgs{
				Method: "echo",
				CallOptions: &tchannel.CallOptions{
					RequestState: rs,
				},
			})
			return err
		})
	}

	preferred, err := client.Peers().Get(nil)
	require.NoError(t, err, "Failed to get peer")
	if preferred.HostPort() == s2.PeerInfo().HostPort {
		// To make the test easier, we want "s1" to always be the preferred hostPort.
		s1, s1Count, s2, s2Count = s2, s2Count, s1, s1Count
	}

	// When we make 10 calls, all of them should go to s1
	for i := 0; i < 10; i++ {
		err := callEcho()
		require.NoError(t, err, "Failed to call echo initially")
	}
	assert.EqualValues(t, 10, s1Count.Load(), "All calls should go to s1")

	// Stop s1, and ensure the client notices S1 has failed.
	s1.Close()
	testutils.WaitFor(time.Second, func() bool {
		if !s1.Closed() {
			return false
		}
		ctx, cancel := tchannel.NewContext(time.Second)
		defer cancel()
		return client.Ping(ctx, s1.PeerInfo().HostPort) != nil
	})

	// Since s1 is stopped, next call should go to s2.
	err = callEcho()
	require.NoError(t, err, "Failed to call echo after s1 close")
	assert.EqualValues(t, 10, s1Count.Load(), "s1 should not get new calls as it's down")
	assert.EqualValues(t, 1, s2Count.Load(), "New call should go to s2")

	// And if s1 comes back, calls should resume to s1.
	s1Up := testutils.NewClient(t, sOpts)
	testutils.RegisterEcho(s1Up, func() { s1Count.Inc() })
	err = s1Up.ListenAndServe(s1.PeerInfo().HostPort)
	require.NoError(t, err, "Failed to bring up a new channel as s1")

	for i := 0; i < 10; i++ {
		require.NoError(t, callEcho(), "Failed to call echo after s1 restarted")
	}
	assert.EqualValues(t, 20, s1Count.Load(), "Once s1 is up, calls should resume to s1")
	assert.EqualValues(t, 1, s2Count.Load(), "s2 should not receive calls after s1 restarted")
}

func stdFnv32a(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func TestFnv32a(t *testing.T) {
	tests := []string{
		"",
		"1.1.1.1",
		"some-other-data",
	}

	for _, tt := range tests {
		assert.Equal(t, stdFnv32a(tt), fnv32a(tt), "Different results for %q", tt)
	}
}

func BenchmarkHrwScoreCalc(b *testing.B) {
	client := testutils.NewClient(b, nil)
	peer := client.Peers().GetOrAdd("1.1.1.1")

	c := NewHRWScorer(1)
	for i := 0; i < b.N; i++ {
		c.GetScore(peer)
	}
}
