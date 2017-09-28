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

package testutils

import (
	"encoding/base32"
	"encoding/binary"
	"math/rand"
	"sync"
)

// This file contains functions for tests to access internal tchannel state.
// Since it has a _test.go suffix, it is only compiled with tests in this package.

var (
	randCache []byte
	randMut   sync.RWMutex
)

func checkCacheSize(n int) {
	// Start with a reasonably large cache.
	if n < 1024 {
		n = 1024
	}

	randMut.RLock()
	curSize := len(randCache)
	randMut.RUnlock()

	// The cache needs to be at least twice as large as the requested size.
	if curSize >= n*2 {
		return
	}

	resizeCache(n)
}

func resizeCache(n int) {
	randMut.Lock()
	defer randMut.Unlock()

	// Double check under the write lock
	if len(randCache) >= n*2 {
		return
	}

	newSize := (n * 2 / 8) * 8
	newCache := make([]byte, newSize)
	copied := copy(newCache, randCache)
	for i := copied; i < newSize; i += 8 {
		n := rand.Int63()
		binary.BigEndian.PutUint64(newCache[i:], uint64(n))
	}
	randCache = newCache
}

// RandBytes returns n random byte slice that points to a shared random byte array.
// Since the underlying random array is shared, the returned byte slice must NOT be modified.
func RandBytes(n int) []byte {
	const maxSize = 2 * 1024 * 1024
	data := make([]byte, 0, n)
	for i := 0; i < n; i += maxSize {
		s := n - i
		if s > maxSize {
			s = maxSize
		}
		data = append(data, randBytes(s)...)
	}
	return data
}

// RandString returns a random alphanumeric string for testing.
func RandString(n int) string {
	encoding := base32.StdEncoding
	numBytes := encoding.DecodedLen(n) + 5
	return base32.StdEncoding.EncodeToString(RandBytes(numBytes))[:n]
}

func randBytes(n int) []byte {
	checkCacheSize(n)

	randMut.RLock()
	startAt := rand.Intn(len(randCache) - n)
	bs := randCache[startAt : startAt+n]
	randMut.RUnlock()
	return bs
}
