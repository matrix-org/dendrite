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
	"runtime"
	"testing"
	"time"

	. "github.com/uber/tchannel-go"

	"github.com/uber/tchannel-go/testutils"
)

func waitForChannelClose(t *testing.T, ch *Channel) bool {
	// TODO: remove standalone use (outside testutils.TestServer).
	started := time.Now()

	var state ChannelState
	for i := 0; i < 50; i++ {
		if state = ch.State(); state == ChannelClosed {
			return true
		}

		runtime.Gosched()
		if i < 5 {
			continue
		}

		sleepFor := time.Duration(i) * 100 * time.Microsecond
		time.Sleep(testutils.Timeout(sleepFor))
	}

	// Channel is not closing, fail the test.
	sinceStart := time.Since(started)
	t.Errorf("Channel did not close after %v, last state: %v", sinceStart, state)
	return false
}

// WithVerifiedServer runs the given test function with a server channel that is verified
// at the end to make sure there are no leaks (e.g. no exchanges leaked).
func WithVerifiedServer(t *testing.T, opts *testutils.ChannelOpts, f func(serverCh *Channel, hostPort string)) {
	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		f(ts.Server(), ts.HostPort())
	})
}
