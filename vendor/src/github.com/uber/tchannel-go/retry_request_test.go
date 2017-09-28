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
	"testing"
	"time"

	. "github.com/uber/tchannel-go"

	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestRequestStateRetry(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		ts.Register(raw.Wrap(newTestHandler(t)), "echo")

		closedHostPorts := make([]string, 4)
		for i := range closedHostPorts {
			hostPort, close := testutils.GetAcceptCloseHostPort(t)
			defer close()
			closedHostPorts[i] = hostPort
		}

		// Since we close connections remotely, there will be some warnings that we can ignore.
		opts := testutils.NewOpts().DisableLogVerification()
		client := ts.NewClient(opts)
		defer client.Close()
		counter := 0

		sc := client.GetSubChannel(ts.Server().ServiceName())
		err := client.RunWithRetry(ctx, func(ctx context.Context, rs *RequestState) error {
			defer func() { counter++ }()

			expectedPeers := counter
			if expectedPeers > 0 {
				// An entry is also added for each host.
				expectedPeers++
			}

			assert.Equal(t, expectedPeers, len(rs.SelectedPeers), "SelectedPeers should not be reused")

			if counter < 4 {
				client.Peers().Add(closedHostPorts[counter])
			} else {
				client.Peers().Add(ts.HostPort())
			}

			_, err := raw.CallV2(ctx, sc, raw.CArgs{
				Method:      "echo",
				CallOptions: &CallOptions{RequestState: rs},
			})
			return err
		})
		assert.NoError(t, err, "RunWithRetry should succeed")
		assert.Equal(t, 5, counter, "RunWithRetry should retry 5 times")
	})
}
