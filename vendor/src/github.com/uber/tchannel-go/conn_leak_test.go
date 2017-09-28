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
package tchannel_test

import (
	"io/ioutil"
	"runtime"
	"testing"
	"time"

	. "github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/require"
)

// This is a regression test for https://github.com/uber/tchannel-go/issues/643
// We want to ensure that once a connection is closed, there are no references
// to the closed connection, and the GC frees the connection.
// We use `runtime.SetFinalizer` to detect whether the GC has freed the object.
// However, finalizers cannot be set on objects with circular references,
// so we cannot set a finalizer on the connection, but instead set a finalizer
// on a field of the connection which has the same lifetime. The connection
// logger is unique per connection and does not have circular references
// so we can use the logger, but need a pointer for `runtime.SetFinalizer`.
// loggerPtr is a Logger implementation that uses a pointer unlike other
// TChannel loggers.
type loggerPtr struct {
	Logger
}

func (l *loggerPtr) WithFields(fields ...LogField) Logger {
	return &loggerPtr{l.Logger.WithFields(fields...)}
}

func TestPeerConnectionLeaks(t *testing.T) {
	// Disable log verification since we want to set our own logger.
	opts := testutils.NewOpts().NoRelay().DisableLogVerification()
	opts.Logger = &loggerPtr{NullLogger}

	connFinalized := make(chan struct{})
	setFinalizer := func(p *Peer, hostPort string) {
		ctx, cancel := NewContext(time.Second)
		defer cancel()

		conn, err := p.GetConnection(ctx)
		require.NoError(t, err, "Failed to get connection")

		runtime.SetFinalizer(conn.Logger(), func(interface{}) {
			close(connFinalized)
		})
	}

	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		s2Opts := testutils.NewOpts().SetServiceName("s2")
		s2Opts.Logger = NewLogger(ioutil.Discard)
		s2 := ts.NewServer(s2Opts)

		// Set a finalizer to detect when the connection from s1 -> s2 is freed.
		peer := ts.Server().Peers().GetOrAdd(s2.PeerInfo().HostPort)
		setFinalizer(peer, s2.PeerInfo().HostPort)

		// Close s2, so that the connection in s1 to s2 is released.
		s2.Close()
		closed := testutils.WaitFor(time.Second, s2.Closed)
		require.True(t, closed, "s2 didn't close")

		// Trigger the GC which will call the finalizer, and ensure
		// that the connection logger was finalized.
		finalized := testutils.WaitFor(time.Second, func() bool {
			runtime.GC()
			select {
			case <-connFinalized:
				return true
			default:
				return false
			}
		})
		require.True(t, finalized, "Connection was not freed")
	})
}
