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

// This file contains functions for tests to access internal tchannel state.
// Since it has a _test.go suffix, it is only compiled with tests in this package.

import (
	"bytes"
	"io"
	"math/rand"
	"sync"
	"testing"
	"time"

	. "github.com/uber/tchannel-go"

	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"
	"github.com/uber/tchannel-go/testutils/goroutines"
	"github.com/uber/tchannel-go/testutils/testreader"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

type swapper struct {
	t *testing.T
}

func (s *swapper) OnError(ctx context.Context, err error) {
	s.t.Errorf("OnError: %v", err)
}

func (*swapper) Handle(ctx context.Context, args *raw.Args) (*raw.Res, error) {
	return &raw.Res{
		Arg2: args.Arg3,
		Arg3: args.Arg2,
	}, nil
}

func doPingAndCall(t *testing.T, clientCh *Channel, hostPort string) {
	ctx, cancel := NewContext(time.Second * 5)
	defer cancel()

	require.NoError(t, clientCh.Ping(ctx, hostPort))

	const maxRandArg = 512 * 1024

	arg2 := testutils.RandBytes(rand.Intn(maxRandArg))
	arg3 := testutils.RandBytes(rand.Intn(maxRandArg))
	resArg2, resArg3, _, err := raw.Call(ctx, clientCh, hostPort, "swap-server", "swap", arg2, arg3)
	if !assert.NoError(t, err, "error during sendRecv") {
		return
	}

	// We expect the arguments to be swapped.
	if bytes.Compare(arg3, resArg2) != 0 {
		t.Errorf("returned arg2 does not match expected:\n  got %v\n want %v", resArg2, arg3)
	}
	if bytes.Compare(arg2, resArg3) != 0 {
		t.Errorf("returned arg2 does not match expected:\n  got %v\n want %v", resArg3, arg2)
	}
}

func doErrorCall(t *testing.T, clientCh *Channel, hostPort string) {
	ctx, cancel := NewContext(time.Second * 5)
	defer cancel()

	_, _, _, err := raw.Call(ctx, clientCh, hostPort, "swap-server", "non-existent", nil, nil)
	assert.Error(t, err, "Call to non-existent endpoint should fail")
	assert.Equal(t, ErrCodeBadRequest, GetSystemErrorCode(err), "Error code mismatch")
}

func TestFramesReleased(t *testing.T) {
	CheckStress(t)

	defer testutils.SetTimeout(t, 30*time.Second)()
	const (
		requestsPerGoroutine = 10
		numGoroutines        = 10
	)

	pool := NewRecordingFramePool()
	opts := testutils.NewOpts().
		SetServiceName("swap-server").
		SetFramePool(pool).
		AddLogFilter("Couldn't find handler.", 2*numGoroutines*requestsPerGoroutine)

	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		ts.Register(raw.Wrap(&swapper{t}), "swap")

		clientOpts := testutils.NewOpts().SetFramePool(pool)
		clientCh := ts.NewClient(clientOpts)

		// Create an active connection that can be shared by the goroutines by calling Ping.
		ctx, cancel := NewContext(time.Second)
		defer cancel()
		require.NoError(t, clientCh.Ping(ctx, ts.HostPort()))

		var wg sync.WaitGroup
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				for i := 0; i < requestsPerGoroutine; i++ {
					doPingAndCall(t, clientCh, ts.HostPort())
					doErrorCall(t, clientCh, ts.HostPort())
				}
			}()
		}

		wg.Wait()
	})

	// TODO: The goroutines.GetAll is to debug test failures in Travis. Remove this once
	// we confirm that the test is not flaky.
	stacks := goroutines.GetAll()
	if unreleasedCount, isEmpty := pool.CheckEmpty(); isEmpty != "" || unreleasedCount > 0 {
		t.Errorf("Frame pool has %v unreleased frames, errors:\n%v\nStacks:%v",
			unreleasedCount, isEmpty, stacks)
	}
}

type dirtyFramePool struct{}

func (p dirtyFramePool) Get() *Frame {
	f := NewFrame(MaxFramePayloadSize)
	reader := testreader.Looper([]byte{^byte(0)})
	io.ReadFull(reader, f.Payload)
	return f
}

func (p dirtyFramePool) Release(f *Frame) {}

func TestDirtyFrameRequests(t *testing.T) {
	argSizes := []int{25000, 50000, 75000}

	// Create the largest required random cache.
	testutils.RandBytes(argSizes[len(argSizes)-1])

	opts := testutils.NewOpts().
		SetServiceName("swap-server").
		SetFramePool(dirtyFramePool{})

	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		ts.Register(raw.Wrap(&swapper{t}), "swap")

		for _, argSize := range argSizes {
			ctx, cancel := NewContext(time.Second)
			defer cancel()

			arg2, arg3 := testutils.RandBytes(argSize), testutils.RandBytes(argSize)
			res2, res3, _, err := raw.Call(ctx, ts.Server(), ts.HostPort(), ts.Server().ServiceName(), "swap", arg2, arg3)
			if assert.NoError(t, err, "Call failed") {
				assert.Equal(t, arg2, res3, "Result arg3 wrong")
				assert.Equal(t, arg3, res2, "Result arg3 wrong")
			}
		}
	})
}
