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
	"math/rand"
	"sync"
	"testing"

	. "github.com/uber/tchannel-go"

	"github.com/uber-go/atomic"
)

func benchmarkUsing(b *testing.B, pool FramePool) {
	const numGoroutines = 1000
	const maxHoldFrames = 1000

	var gotFrames atomic.Uint64

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {

			for {
				if gotFrames.Load() > uint64(b.N) {
					break
				}

				framesToHold := rand.Intn(maxHoldFrames)
				gotFrames.Add(uint64(framesToHold))

				frames := make([]*Frame, framesToHold)
				for i := 0; i < framesToHold; i++ {
					frames[i] = pool.Get()
				}

				for i := 0; i < framesToHold; i++ {
					pool.Release(frames[i])
				}
			}

			wg.Done()
		}()
	}

	wg.Wait()
}

func BenchmarkFramePoolDisabled(b *testing.B) {
	benchmarkUsing(b, DisabledFramePool)
}

func BenchmarkFramePoolSync(b *testing.B) {
	benchmarkUsing(b, NewSyncFramePool())
}

func BenchmarkFramePoolChannel1000(b *testing.B) {
	benchmarkUsing(b, NewChannelFramePool(1000))
}

func BenchmarkFramePoolChannel10000(b *testing.B) {
	benchmarkUsing(b, NewChannelFramePool(10000))
}
