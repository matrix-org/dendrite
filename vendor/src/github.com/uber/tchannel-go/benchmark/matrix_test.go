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
	"fmt"
	"testing"

	"github.com/uber/tchannel-go"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// combinations will call f with every combination of selecting elements
// from a slice with the specified length.
// e.g. for 2, the callback would be:
// f(false, false)
// f(false, true)
// f(true, false)
// f(true, true)
func combinations(length int, f func([]bool)) {
	cur := make([]bool, length)
	toGenerate := (1 << uint(length))

	f(cur)
	for i := 0; i < toGenerate-1; i++ {
		var digit int
		for digit = length - 1; cur[digit]; digit-- {
			cur[digit] = false
		}
		cur[digit] = true
		f(cur)
	}
}

func TestCombinations(t *testing.T) {
	tests := []struct {
		length int
		want   [][]bool
	}{
		{
			length: 1,
			want:   [][]bool{{false}, {true}},
		},
		{
			length: 2,
			want:   [][]bool{{false, false}, {false, true}, {true, false}, {true, true}},
		},
	}

	for _, tt := range tests {
		var got [][]bool
		recordCombs := func(comb []bool) {
			copied := append([]bool(nil), comb...)
			got = append(got, copied)
		}
		combinations(tt.length, recordCombs)
		assert.Equal(t, tt.want, got, "Mismatch for combinations of length %v", tt.length)
	}
}

func selectOptions(options []Option, toSelect []bool) []Option {
	var opts []Option
	for i, v := range toSelect {
		if v {
			opts = append(opts, options[i])
		}
	}
	return opts
}

func combineOpts(base, override []Option) []Option {
	resultOpts := append([]Option(nil), base...)
	return append(resultOpts, override...)
}

func runSingleTest(t *testing.T, baseOpts, serverOpts, clientOpts []Option) {
	serverOpts = combineOpts(baseOpts, serverOpts)
	clientOpts = combineOpts(baseOpts, clientOpts)

	msgP := fmt.Sprintf("%+v: ", struct {
		serverOpts options
		clientOpts options
	}{*(getOptions(serverOpts)), *(getOptions(clientOpts))})

	server := NewServer(serverOpts...)
	defer server.Close()

	client := NewClient([]string{server.HostPort()}, clientOpts...)
	defer client.Close()

	require.NoError(t, client.Warmup(), msgP+"Client warmup failed")

	durations, err := client.RawCall(0)
	require.NoError(t, err, msgP+"Call(0) failed")
	assert.Equal(t, 0, len(durations), msgP+"Wrong number of calls")

	assert.Equal(t, 0, server.RawCalls(), msgP+"server.RawCalls mismatch")
	assert.Equal(t, 0, server.ThriftCalls(), msgP+"server.ThriftCalls mismatch")

	expectCalls := 0
	for i := 1; i < 10; i *= 2 {
		durations, err = client.RawCall(i)
		require.NoError(t, err, msgP+"Call(%v) failed", i)
		require.Equal(t, i, len(durations), msgP+"Wrong number of calls")

		expectCalls += i
		require.Equal(t, expectCalls, server.RawCalls(), msgP+"server.RawCalls mismatch")
		require.Equal(t, 0, server.ThriftCalls(), msgP+"server.ThriftCalls mismatch")
	}
}

func TestServerClientMatrix(t *testing.T) {
	tests := [][]Option{
		{WithServiceName("other")},
		{WithRequestSize(tchannel.MaxFrameSize)},
	}

	// These options can be independently applied to the server or the client.
	independentOpts := []Option{
		WithExternalProcess(),
		WithNoLibrary(),
	}

	// These options only apply to the client.
	clientOnlyOpts := combineOpts(independentOpts, []Option{
		WithNumClients(5),
	})

	for _, tt := range tests {
		combinations(len(independentOpts), func(serverSelect []bool) {
			combinations(len(clientOnlyOpts), func(clientSelect []bool) {
				serverOpts := selectOptions(independentOpts, serverSelect)
				clientOpts := selectOptions(clientOnlyOpts, clientSelect)
				runSingleTest(t, tt, serverOpts, clientOpts)
			})
		})
	}
}
