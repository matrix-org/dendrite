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
	"testing"

	"github.com/uber/tchannel-go"

	"github.com/stretchr/testify/require"
)

func updateOptsLogger(opts *ChannelOpts) {
	if opts.Logger == nil && *connectionLog {
		opts.Logger = tchannel.SimpleLogger
	}
}

func updateOptsForTest(t testing.TB, opts *ChannelOpts) {
	updateOptsLogger(opts)

	// If there's no logger, then register the test logger which will record
	// everything to a buffer, and print out the buffer if the test fails.
	if opts.Logger == nil {
		tl := newTestLogger(t)
		opts.Logger = tl
		opts.addPostFn(tl.report)
	}

	if !opts.LogVerification.Disabled {
		opts.Logger = opts.LogVerification.WrapLogger(t, opts.Logger)
	}
}

// WithServer sets up a TChannel that is listening and runs the given function with the channel.
func WithServer(t testing.TB, opts *ChannelOpts, f func(ch *tchannel.Channel, hostPort string)) {
	opts = opts.Copy()
	updateOptsForTest(t, opts)
	ch := NewServer(t, opts)
	f(ch, ch.PeerInfo().HostPort)
	ch.Close()
}

// NewServer returns a new TChannel server that listens on :0.
func NewServer(t testing.TB, opts *ChannelOpts) *tchannel.Channel {
	return newServer(t, opts.Copy())
}

// newServer must be passed non-nil opts that may be mutated to include
// post-verification steps.
func newServer(t testing.TB, opts *ChannelOpts) *tchannel.Channel {
	updateOptsForTest(t, opts)
	ch, err := NewServerChannel(opts)
	require.NoError(t, err, "NewServerChannel failed")
	return ch
}

// NewClient returns a new TChannel that is not listening.
func NewClient(t testing.TB, opts *ChannelOpts) *tchannel.Channel {
	return newClient(t, opts.Copy())
}

// newClient must be passed non-nil opts that may be mutated to include
// post-verification steps.
func newClient(t testing.TB, opts *ChannelOpts) *tchannel.Channel {
	updateOptsForTest(t, opts)
	ch, err := NewClientChannel(opts)
	require.NoError(t, err, "NewServerChannel failed")
	return ch
}
