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

// +build go1.7

package tchannel_test

import (
	"strings"
	"testing"
	"time"

	. "github.com/uber/tchannel-go"

	"github.com/stretchr/testify/assert"
	"github.com/uber/tchannel-go/testutils"
)

func TestNetDialCancelContext(t *testing.T) {
	// timeoutHostPort uses a blackholed address (RFC 6890) with a port
	// reserved for documentation. This address should always cause a timeout.
	const timeoutHostPort = "192.18.0.254:44444"
	timeoutPeriod := testutils.Timeout(50 * time.Millisecond)

	client := testutils.NewClient(t, nil)
	defer client.Close()

	started := time.Now()
	ctx, cancel := NewContext(time.Minute)

	go func() {
		time.Sleep(timeoutPeriod)
		cancel()
	}()

	err := client.Ping(ctx, timeoutHostPort)
	if !assert.Error(t, err, "Ping to blackhole address should fail") {
		return
	}

	if strings.Contains(err.Error(), "network is unreachable") {
		t.Skipf("Skipping test, as network interface may not be available")
	}

	d := time.Since(started)
	assert.Equal(t, ErrCodeCancelled, GetSystemErrorCode(err), "Ping expected to fail with context cancelled")
	assert.True(t, d < 2*timeoutPeriod, "Timeout should take less than %v, took %v", 2*timeoutPeriod, d)
}
