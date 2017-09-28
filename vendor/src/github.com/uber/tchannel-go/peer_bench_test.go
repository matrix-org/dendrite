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
	"testing"
	"time"

	. "github.com/uber/tchannel-go"

	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/require"
)

func benchmarkGetConnection(b *testing.B, numIncoming, numOutgoing int) {
	ctx, cancel := NewContext(10 * time.Second)
	defer cancel()

	s1 := testutils.NewServer(b, nil)
	s2 := testutils.NewServer(b, nil)
	defer s1.Close()
	defer s2.Close()

	for i := 0; i < numOutgoing; i++ {
		_, err := s1.Connect(ctx, s2.PeerInfo().HostPort)
		require.NoError(b, err, "Connect from s1 -> s2 failed")
	}
	for i := 0; i < numIncoming; i++ {
		_, err := s2.Connect(ctx, s1.PeerInfo().HostPort)
		require.NoError(b, err, "Connect from s2 -> s1 failed")
	}

	peer := s1.Peers().GetOrAdd(s2.PeerInfo().HostPort)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.GetConnection(ctx)
	}
}

func BenchmarkGetConnection0In1Out(b *testing.B) { benchmarkGetConnection(b, 0, 1) }
func BenchmarkGetConnection1In0Out(b *testing.B) { benchmarkGetConnection(b, 1, 0) }
func BenchmarkGetConnection5In5Out(b *testing.B) { benchmarkGetConnection(b, 5, 5) }
