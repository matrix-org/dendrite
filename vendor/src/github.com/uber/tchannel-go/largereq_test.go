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
	"bytes"
	"log"
	"testing"
	"time"

	. "github.com/uber/tchannel-go"

	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/require"
)

func TestLargeRequest(t *testing.T) {
	CheckStress(t)

	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB

		maxRequestSize = 1 * GB
	)

	WithVerifiedServer(t, nil, func(serverCh *Channel, hostPort string) {
		serverCh.Register(raw.Wrap(newTestHandler(t)), "echo")

		for reqSize := 2; reqSize <= maxRequestSize; reqSize *= 2 {
			log.Printf("reqSize = %v", reqSize)
			arg3 := testutils.RandBytes(reqSize)
			arg2 := testutils.RandBytes(reqSize / 2)

			clientCh := testutils.NewClient(t, nil)
			ctx, cancel := NewContext(time.Second * 30)
			rArg2, rArg3, _, err := raw.Call(ctx, clientCh, hostPort, serverCh.PeerInfo().ServiceName, "echo", arg2, arg3)
			require.NoError(t, err, "Call failed")

			if !bytes.Equal(arg2, rArg2) {
				t.Errorf("echo arg2 mismatch")
			}
			if !bytes.Equal(arg3, rArg3) {
				t.Errorf("echo arg3 mismatch")
			}
			cancel()
		}
	})
}
