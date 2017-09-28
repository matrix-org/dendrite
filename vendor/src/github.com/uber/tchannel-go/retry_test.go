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
	"net"
	"testing"
	"time"

	. "github.com/uber/tchannel-go"

	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func createFuncToRetry(t *testing.T, errors ...error) (RetriableFunc, *int) {
	i := 0
	return func(_ context.Context, rs *RequestState) error {
		defer func() { i++ }()
		if i >= len(errors) {
			t.Fatalf("Retry function has no error to return for this call")
		}
		assert.Equal(t, i+1, rs.Attempt, "Attempt count mismatch")

		err := errors[i]
		return err
	}, &i
}

type testErrors struct {
	Busy       error
	Declined   error
	Timeout    error
	Network    error
	Connection error
	BadRequest error
	Unexpected error
	Cancelled  error

	all []error
}

func getTestErrors() testErrors {
	errs := testErrors{
		Busy:       ErrServerBusy,
		Declined:   ErrChannelClosed,
		Timeout:    ErrTimeout,
		Network:    NewSystemError(ErrCodeNetwork, "fake network error"),
		Connection: net.UnknownNetworkError("fake connection error"),
		BadRequest: ErrTimeoutRequired,
		Unexpected: NewSystemError(ErrCodeUnexpected, "fake unexpected error"),
		Cancelled:  NewSystemError(ErrCodeCancelled, "fake cancelled error"),
	}
	errs.all = []error{errs.Busy, errs.Declined, errs.Timeout, errs.Network, errs.Connection,
		errs.BadRequest, errs.Unexpected, errs.Cancelled}
	return errs
}

func TestCanRetry(t *testing.T) {
	e := getTestErrors()
	tests := []struct {
		RetryOn RetryOn
		RetryOK []error
	}{
		{RetryNever, nil},
		{RetryDefault, []error{e.Busy, e.Declined, e.Network, e.Connection}},
		{RetryConnectionError, []error{e.Busy, e.Declined, e.Network, e.Connection}},
		{RetryNonIdempotent, []error{e.Busy, e.Declined}},
		{RetryUnexpected, []error{e.Busy, e.Declined, e.Unexpected}},
		{RetryIdempotent, []error{e.Busy, e.Declined, e.Timeout, e.Network, e.Connection, e.Unexpected, e.Cancelled}},
	}

	for _, tt := range tests {
		retryOK := make(map[error]bool)
		for _, err := range tt.RetryOK {
			retryOK[err] = true
		}

		for _, err := range e.all {
			expectOK := retryOK[err]
			assert.Equal(t, expectOK, tt.RetryOn.CanRetry(err),
				"%v.CanRetry(%v) expected %v", tt.RetryOn, err, expectOK)
		}
	}
}

func TestNoRetry(t *testing.T) {
	ch := testutils.NewClient(t, nil)
	defer ch.Close()

	e := getTestErrors()
	retryOpts := &RetryOptions{RetryOn: RetryNever}
	for _, fErr := range e.all {
		ctx, cancel := NewContextBuilder(time.Second).SetRetryOptions(retryOpts).Build()
		defer cancel()

		f, counter := createFuncToRetry(t, fErr)
		err := ch.RunWithRetry(ctx, f)
		assert.Equal(t, fErr, err)
		assert.Equal(t, 1, *counter, "f should not be retried when retried are disabled")
	}
}

func TestRetryTillMaxAttempts(t *testing.T) {
	ch := testutils.NewClient(t, nil)
	defer ch.Close()

	setErr := ErrServerBusy
	runTest := func(maxAttempts, numErrors, expectCounter int, expectErr error) {
		retryOpts := &RetryOptions{MaxAttempts: maxAttempts}
		ctx, cancel := NewContextBuilder(time.Second).SetRetryOptions(retryOpts).Build()
		defer cancel()

		var errors []error
		for i := 0; i < numErrors; i++ {
			errors = append(errors, setErr)
		}
		errors = append(errors, nil)

		f, counter := createFuncToRetry(t, errors...)
		err := ch.RunWithRetry(ctx, f)
		assert.Equal(t, expectErr, err,
			"unexpected result for maxAttempts = %v numErrors = %v", maxAttempts, numErrors)
		assert.Equal(t, expectCounter, *counter,
			"expected f to be retried %v times with maxAttempts = %v numErrors = %v",
			expectCounter, maxAttempts, numErrors)
	}

	for numAttempts := 1; numAttempts < 5; numAttempts++ {
		for numErrors := 0; numErrors < numAttempts+3; numErrors++ {
			var expectErr error
			if numErrors >= numAttempts {
				expectErr = setErr
			}

			expectCount := numErrors + 1
			if expectCount > numAttempts {
				expectCount = numAttempts
			}

			runTest(numAttempts, numErrors, expectCount, expectErr)
		}
	}
}

func TestRetrySubContextNoTimeoutPerAttempt(t *testing.T) {
	e := getTestErrors()
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	ch := testutils.NewClient(t, nil)
	defer ch.Close()

	counter := 0
	ch.RunWithRetry(ctx, func(sctx context.Context, _ *RequestState) error {
		counter++
		assert.Equal(t, ctx, sctx, "Sub-context should be the same")
		return e.Busy
	})
	assert.Equal(t, 5, counter, "RunWithRetry did not run f enough times")
}

func TestRetrySubContextTimeoutPerAttempt(t *testing.T) {
	e := getTestErrors()
	ctx, cancel := NewContextBuilder(time.Second).
		SetTimeoutPerAttempt(time.Millisecond).Build()
	defer cancel()

	ch := testutils.NewClient(t, nil)
	defer ch.Close()

	var lastDeadline time.Time

	counter := 0
	ch.RunWithRetry(ctx, func(sctx context.Context, _ *RequestState) error {
		counter++

		assert.NotEqual(t, ctx, sctx, "Sub-context should be different")
		deadline, _ := sctx.Deadline()
		assert.True(t, deadline.After(lastDeadline), "Deadline is invalid")
		lastDeadline = deadline

		overallDeadline, _ := ctx.Deadline()
		assert.True(t, overallDeadline.After(deadline), "Deadline is invalid")

		return e.Busy
	})
	assert.Equal(t, 5, counter, "RunWithRetry did not run f enough times")
}

func TestRetryNetConnect(t *testing.T) {
	e := getTestErrors()
	ch := testutils.NewClient(t, nil)
	defer ch.Close()

	ctx, cancel := NewContext(time.Second)
	defer cancel()

	closedAddr := testutils.GetClosedHostPort(t)
	listenC, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Listen failed")
	defer listenC.Close()

	counter := 0
	f := func(ctx context.Context, rs *RequestState) error {
		counter++
		if !rs.HasRetries(e.Connection) {
			c, err := net.Dial("tcp", listenC.Addr().String())
			if err == nil {
				c.Close()
			}
			return err
		}

		_, err := net.Dial("tcp", closedAddr)
		return err
	}

	assert.NoError(t, ch.RunWithRetry(ctx, f), "RunWithRetry should succeed")
	assert.Equal(t, 5, counter, "RunWithRetry should have run f 5 times")
}

func TestRequestStateSince(t *testing.T) {
	baseTime := time.Date(2015, 1, 2, 3, 4, 5, 6, time.UTC)
	tests := []struct {
		requestState *RequestState
		now          time.Time
		fallback     time.Duration
		expected     time.Duration
	}{
		{
			requestState: nil,
			fallback:     3 * time.Millisecond,
			expected:     3 * time.Millisecond,
		},
		{
			requestState: &RequestState{Start: baseTime},
			now:          baseTime.Add(7 * time.Millisecond),
			fallback:     5 * time.Millisecond,
			expected:     7 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		got := tt.requestState.SinceStart(tt.now, tt.fallback)
		assert.Equal(t, tt.expected, got, "%+v.SinceStart(%v, %v) expected %v got %v",
			tt.requestState, tt.now, tt.fallback, tt.expected, got)
	}
}
