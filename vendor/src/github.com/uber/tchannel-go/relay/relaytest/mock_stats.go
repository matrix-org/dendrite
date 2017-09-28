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

package relaytest

import (
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/tchannel-go/relay"
)

// MockCallStats is a testing spy for the CallStats interface.
type MockCallStats struct {
	// Store ints and slices instead of bools and strings so that we can assert
	// the actual sequence of calls (in case we expect to call both Succeeded
	// and Failed). The real implementation will have the first writer win.
	succeeded  int
	failedMsgs []string
	ended      int
	wg         *sync.WaitGroup
}

// Succeeded marks the RPC as succeeded.
func (m *MockCallStats) Succeeded() {
	m.succeeded++
}

// Failed marks the RPC as failed for the provided reason.
func (m *MockCallStats) Failed(reason string) {
	m.failedMsgs = append(m.failedMsgs, reason)
}

// End halts timer and metric collection for the RPC.
func (m *MockCallStats) End() {
	m.ended++
	m.wg.Done()
}

// FluentMockCallStats wraps the MockCallStats in a fluent API that's convenient for tests.
type FluentMockCallStats struct {
	*MockCallStats
}

// Succeeded marks the RPC as succeeded.
func (f *FluentMockCallStats) Succeeded() *FluentMockCallStats {
	f.MockCallStats.Succeeded()
	return f
}

// Failed marks the RPC as failed.
func (f *FluentMockCallStats) Failed(reason string) *FluentMockCallStats {
	f.MockCallStats.Failed(reason)
	return f
}

// MockStats is a testing spy for the Stats interface.
type MockStats struct {
	mu    sync.Mutex
	wg    sync.WaitGroup
	stats map[string][]*MockCallStats
}

// NewMockStats constructs a MockStats.
func NewMockStats() *MockStats {
	return &MockStats{
		stats: make(map[string][]*MockCallStats),
	}
}

// Begin starts collecting metrics for an RPC.
func (m *MockStats) Begin(f relay.CallFrame) *MockCallStats {
	return m.Add(string(f.Caller()), string(f.Service()), string(f.Method())).MockCallStats
}

// Add explicitly adds a new call along an edge of the call graph.
func (m *MockStats) Add(caller, callee, procedure string) *FluentMockCallStats {
	m.wg.Add(1)
	cs := &MockCallStats{wg: &m.wg}
	key := m.tripleToKey(caller, callee, procedure)
	m.mu.Lock()
	m.stats[key] = append(m.stats[key], cs)
	m.mu.Unlock()
	return &FluentMockCallStats{cs}
}

// AssertEqual asserts that two MockStats describe the same call graph.
func (m *MockStats) AssertEqual(t testing.TB, expected *MockStats) {
	// Wait for any outstandanding CallStats to end.
	m.wg.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	expected.mu.Lock()
	defer expected.mu.Unlock()

	if assert.Equal(t, getEdges(expected.stats), getEdges(m.stats), "Found calls along unexpected edges.") {
		for edge := range expected.stats {
			m.assertEdgeEqual(t, expected, edge)
		}
	}
}

func (m *MockStats) assertEdgeEqual(t testing.TB, expected *MockStats, edge string) {
	expectedCalls := expected.stats[edge]
	actualCalls := m.stats[edge]
	if assert.Equal(t, len(expectedCalls), len(actualCalls), "Unexpected number of calls along %s edge.", edge) {
		for i := range expectedCalls {
			m.assertCallEqual(t, expectedCalls[i], actualCalls[i])
		}
	}
}

func (m *MockStats) assertCallEqual(t testing.TB, expected *MockCallStats, actual *MockCallStats) {
	// Revisit these assertions if we ever need to assert zero or many calls to
	// End.
	require.Equal(t, 1, expected.ended, "Expected call must assert exactly one call to End.")
	require.False(
		t,
		expected.succeeded <= 0 && len(expected.failedMsgs) == 0,
		"Expectation must indicate whether RPC should succeed or fail.",
	)

	assert.Equal(t, expected.succeeded, actual.succeeded, "Unexpected number of successes.")
	assert.Equal(t, expected.failedMsgs, actual.failedMsgs, "Unexpected reasons for RPC failure.")
	assert.Equal(t, expected.ended, actual.ended, "Unexpected number of calls to End.")

	if t.Failed() {
		// The default testify output is often insufficient.
		t.Logf("\nExpected relayed stats were:\n\t%+v\nActual relayed stats were:\n\t%+v\n", expected, actual)
	}
}

func (m *MockStats) tripleToKey(caller, callee, procedure string) string {
	return fmt.Sprintf("%s->%s::%s", caller, callee, procedure)
}

func getEdges(m map[string][]*MockCallStats) []string {
	edges := make([]string, 0, len(m))
	for k := range m {
		edges = append(edges, k)
	}
	sort.Strings(edges)
	return edges
}
