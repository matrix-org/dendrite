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

// WaitFor will retry f till it returns true for a maximum of timeout.
// It returns true if f returned true, false if timeout was hit.
func WaitFor(timeout time.Duration, f func() bool) bool {
	timeoutEnd := time.Now().Add(Timeout(timeout))

	const maxSleep = time.Millisecond * 50
	sleepFor := time.Millisecond
	for {
		if f() {
			return true
		}

		if time.Now().After(timeoutEnd) {
			return false
		}

		time.Sleep(sleepFor)
		if sleepFor < maxSleep {
			sleepFor *= 2
		}
	}
}

// WaitWG waits for the given WaitGroup to be complete with a timeout
// and returns whether the WaitGroup completed within the timeout.
func WaitWG(wg *sync.WaitGroup, timeout time.Duration) bool {
	wgC := make(chan struct{})

	go func() {
		wg.Wait()
		wgC <- struct{}{}
	}()
	select {
	case <-time.After(timeout):
		return false
	case <-wgC:
		return true
	}
}
