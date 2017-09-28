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

import "time"

// SleepStub stubs a function variable that points to time.Sleep. It returns
// two channels to control the sleep stub, and a function to close the channels.
// Once the stub is closed, any further sleeps will cause panics.
// The two channels returned are:
// <-chan time.Duration which will contain arguments that the stub was called with.
// chan<- struct{} that should be written to when you want the Sleep to return.
func SleepStub(funcVar *func(time.Duration)) (
	argCh <-chan time.Duration, unblockCh chan<- struct{}, closeFn func()) {

	args := make(chan time.Duration)
	block := make(chan struct{})
	*funcVar = func(t time.Duration) {
		args <- t
		<-block
	}
	closeSleepChans := func() {
		close(args)
		close(block)
	}
	return args, block, closeSleepChans
}

// ResetSleepStub resets a Sleep stub.
func ResetSleepStub(funcVar *func(time.Duration)) {
	*funcVar = time.Sleep
}
