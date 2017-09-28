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

import "github.com/uber/tchannel-go"

type hrwScoreCalc struct {
	clientID uint32
}

// NewHRWScorer returns a ScoreCalculator based on Rendezvous or Highest Random Weight
// hashing.
// It is useful for distributing load in peer-to-peer situations where we have
// many clients picking from a set of servers with "sticky" semantics that will
// spread load evenly as servers go down or new servers are added.
// The clientID is used to score the servers, so each client should pass in
// a unique client ID.
func NewHRWScorer(clientID uint32) tchannel.ScoreCalculator {
	return &hrwScoreCalc{mod2_31(clientID)}
}

func (s *hrwScoreCalc) GetScore(p *tchannel.Peer) uint64 {
	server := mod2_31(fnv32a(p.HostPort()))

	// These constants are taken from W_rand2 in the Rendezvous paper:
	// http://www.eecs.umich.edu/techreports/cse/96/CSE-TR-316-96.pdf
	v := 1103515245*((1103515245*s.clientID+12345)^server) + 12345
	return uint64(mod2_31(v))
}

func mod2_31(v uint32) uint32 {
	return v & ((1 << 31) - 1)
}

// This is based on the standard library's fnv32a implementation.
// We copy it for a couple of reasons:
// 1. Avoid allocations to create a hash.Hash32
// 2. Avoid converting the []byte to a string (another allocation) since
//    the Hash32 interface only supports writing bytes.
func fnv32a(s string) uint32 {
	const (
		initial = 2166136261
		prime   = 16777619
	)

	hash := uint32(initial)
	for i := 0; i < len(s); i++ {
		hash ^= uint32(s[i])
		hash *= prime
	}
	return hash
}
