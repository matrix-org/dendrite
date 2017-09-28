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
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testDecrementor(t *testing.T, f func(dec Decrement) int) {
	const count = 10000
	const numGoroutines = 100

	dec := Decrementor(count)
	results := make(chan int, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			results <- f(dec)
		}()
	}

	var total int
	for i := 0; i < numGoroutines; i++ {
		total += <-results
	}
	assert.Equal(t, count, total, "Count mismatch")
}

func TestDecrementSingle(t *testing.T) {
	testDecrementor(t, func(dec Decrement) int {
		count := 0
		for dec.Single() {
			count++
		}
		return count
	})
}

func TestDecrementMultiple(t *testing.T) {
	testDecrementor(t, func(dec Decrement) int {
		count := 0
		for {
			tokens := dec.Multiple(rand.Intn(100) + 1)
			if tokens == 0 {
				break
			}
			count += tokens
		}
		return count
	})
}

func TestBatch(t *testing.T) {
	tests := []struct {
		n     int
		batch int
		want  []int
	}{
		{40, 10, []int{10, 10, 10, 10}},
		{5, 10, []int{5}},
		{45, 10, []int{10, 10, 10, 10, 5}},
	}

	for _, tt := range tests {
		got := Batch(tt.n, tt.batch)
		assert.Equal(t, tt.want, got, "Batch(%v, %v) unexpected result", tt.n, tt.batch)
	}
}

func TestBuckets(t *testing.T) {
	tests := []struct {
		n       int
		buckets int
		want    []int
	}{
		{2, 3, []int{2, 0, 0}},
		{3, 3, []int{1, 1, 1}},
		{4, 3, []int{2, 1, 1}},
	}

	for _, tt := range tests {
		got := Buckets(tt.n, tt.buckets)
		assert.Equal(t, tt.want, got, "Buckets(%v, %v) unexpected result", tt.n, tt.buckets)
	}
}
