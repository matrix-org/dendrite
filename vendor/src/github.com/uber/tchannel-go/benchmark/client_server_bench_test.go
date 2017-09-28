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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func BenchmarkServer(b *testing.B) {
	server := NewServer()

	client := NewClient([]string{server.HostPort()},
		WithExternalProcess(),
		WithNoLibrary(),
		WithNumClients(10),
		WithNoDurations(),
		WithTimeout(10*time.Second),
	)

	assert.NoError(b, client.Warmup(), "Warmup failed")

	b.ResetTimer()
	started := time.Now()
	_, err := client.RawCall(b.N)
	total := time.Since(started)
	assert.NoError(b, err, "client.RawCall failed")

	if n := server.RawCalls(); b.N > n {
		b.Errorf("Server received %v calls, expected at least %v calls", n, b.N)
	}
	log.Printf("Calls: %v Duration: %v RPS: %.0f", b.N, total, float64(b.N)/total.Seconds())
}

func BenchmarkClient(b *testing.B) {
	servers := make([]Server, 3)
	serverHosts := make([]string, len(servers))
	for i := range servers {
		servers[i] = NewServer(
			WithExternalProcess(),
			WithNoLibrary(),
		)
		serverHosts[i] = servers[i].HostPort()
	}

	// To saturate a single process, we need to have multiple clients.
	client := NewClient(serverHosts,
		WithNoChecking(),
		WithNumClients(10),
	)
	require.NoError(b, client.Warmup(), "Warmup failed")

	b.ResetTimer()
	started := time.Now()
	if _, err := client.RawCall(b.N); err != nil {
		b.Fatalf("Call failed: %v", err)
	}
	total := time.Since(started)
	log.Printf("Calls: %v Duration: %v RPS: %.0f", b.N, total, float64(b.N)/total.Seconds())
}
