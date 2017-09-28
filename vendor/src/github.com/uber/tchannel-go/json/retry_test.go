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

package json

import (
	"strings"
	"testing"
	"time"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryJSONCall(t *testing.T) {
	ch := testutils.NewServer(t, nil)
	ch.Peers().Add(ch.PeerInfo().HostPort)

	count := 0
	handler := func(ctx Context, req map[string]string) (map[string]string, error) {
		count++
		if count > 4 {
			return req, nil
		}
		return nil, tchannel.ErrServerBusy
	}
	Register(ch, Handlers{"test": handler}, nil)

	ctx, cancel := NewContext(time.Second)
	defer cancel()

	client := NewClient(ch, ch.ServiceName(), nil)

	var res map[string]string
	err := client.Call(ctx, "test", nil, &res)
	assert.NoError(t, err, "Call should succeed")
	assert.Equal(t, 5, count, "Handler should have been invoked 5 times")
}

func TestRetryJSONNoConnect(t *testing.T) {
	ch := testutils.NewClient(t, nil)
	ch.Peers().Add("0.0.0.0:0")

	ctx, cancel := NewContext(time.Second)
	defer cancel()

	var res map[string]interface{}
	client := NewClient(ch, ch.ServiceName(), nil)
	err := client.Call(ctx, "test", nil, &res)
	require.Error(t, err, "Call should fail")
	assert.True(t, strings.HasPrefix(err.Error(), "connect: "), "Error does not contain expected prefix: %v", err.Error())
}
