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
	"sync"
	"time"
)

// NowStub returns a stub time.Now function that allows the return values to
// to be controller by the caller. It returns two functions:
// stub: The stub time.Now function.
// increment: Used to control the increment amount between calls.
func NowStub(initial time.Time) (stub func() time.Time, increment func(time.Duration)) {
	var mut sync.Mutex
	cur := initial
	var addAmt time.Duration
	stub = func() time.Time {
		mut.Lock()
		defer mut.Unlock()
		cur = cur.Add(addAmt)
		return cur
	}
	increment = func(d time.Duration) {
		addAmt = d
	}
	return stub, increment
}
