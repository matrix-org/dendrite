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

package tchannel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllChannelsRegistered(t *testing.T) {
	introspectOpts := &IntrospectionOptions{IncludeOtherChannels: true}

	ch1_1, err := NewChannel("ch1", nil)
	require.NoError(t, err, "Channel create failed")
	ch1_2, err := NewChannel("ch1", nil)
	require.NoError(t, err, "Channel create failed")
	ch2_1, err := NewChannel("ch2", nil)
	require.NoError(t, err, "Channel create failed")

	state := ch1_1.IntrospectState(introspectOpts)
	assert.Equal(t, 1, len(state.OtherChannels["ch1"]))
	assert.Equal(t, 1, len(state.OtherChannels["ch2"]))

	ch1_2.Close()

	state = ch1_1.IntrospectState(introspectOpts)
	assert.Equal(t, 0, len(state.OtherChannels["ch1"]))
	assert.Equal(t, 1, len(state.OtherChannels["ch2"]))

	ch2_2, err := NewChannel("ch2", nil)

	state = ch1_1.IntrospectState(introspectOpts)
	require.NoError(t, err, "Channel create failed")
	assert.Equal(t, 0, len(state.OtherChannels["ch1"]))
	assert.Equal(t, 2, len(state.OtherChannels["ch2"]))

	ch1_1.Close()
	ch2_1.Close()
	ch2_2.Close()

	state = ch1_1.IntrospectState(introspectOpts)
	assert.Equal(t, 0, len(state.OtherChannels["ch1"]))
	assert.Equal(t, 0, len(state.OtherChannels["ch2"]))
}
