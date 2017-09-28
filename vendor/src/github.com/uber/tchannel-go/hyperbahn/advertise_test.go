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

package hyperbahn

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/json"
	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInitialAdvertiseFailedRetryBackoff(t *testing.T) {
	defer testutils.SetTimeout(t, 2*time.Second)()

	clientOpts := stubbedSleep()
	sleepArgs, sleepBlock, sleepClose := testutils.SleepStub(&clientOpts.TimeSleep)

	// We expect to retry 5 times,
	go func() {
		for attempt := uint(0); attempt < 5; attempt++ {
			maxSleepFor := advertiseRetryInterval * time.Duration(1<<attempt)
			got := <-sleepArgs
			assert.True(t, got <= maxSleepFor,
				"Initial advertise attempt %v expected sleep %v < %v", attempt, got, maxSleepFor)
			sleepBlock <- struct{}{}
		}
		sleepClose()
	}()

	withSetup(t, func(hypCh *tchannel.Channel, hostPort string) {
		serverCh := testutils.NewServer(t, nil)
		defer serverCh.Close()

		client, err := NewClient(serverCh, configFor(hostPort), clientOpts)
		require.NoError(t, err, "NewClient")
		defer client.Close()
		assert.Error(t, client.Advertise(), "Advertise without handler should fail")
	})
}

func TestInitialAdvertiseFailedRetryTimeout(t *testing.T) {
	withSetup(t, func(hypCh *tchannel.Channel, hyperbahnHostPort string) {
		started := time.Now()
		count := 0
		adHandler := func(ctx json.Context, req *AdRequest) (*AdResponse, error) {
			count++

			deadline, ok := ctx.Deadline()
			if assert.True(t, ok, "context is missing Deadline") {
				assert.True(t, deadline.Sub(started) <= 2*time.Second,
					"Timeout per attempt should be 1 second. Started: %v Deadline: %v", started, deadline)
			}

			return nil, tchannel.NewSystemError(tchannel.ErrCodeUnexpected, "unexpected")
		}
		json.Register(hypCh, json.Handlers{"ad": adHandler}, nil)

		ch := testutils.NewServer(t, nil)
		client, err := NewClient(ch, configFor(hyperbahnHostPort), stubbedSleep())
		assert.NoError(t, err, "hyperbahn NewClient failed")
		defer client.Close()

		assert.Error(t, client.Advertise(), "Advertise should not succeed")
		// We expect 5 retries by TChannel and we attempt 5 to advertise 5 times.
		assert.Equal(t, 5*5, count, "adHandler not retried correct number of times")
	})
}

func TestNotListeningChannel(t *testing.T) {
	withSetup(t, func(hypCh *tchannel.Channel, hyperbahnHostPort string) {
		adHandler := func(ctx json.Context, req *AdRequest) (*AdResponse, error) {
			return &AdResponse{1}, nil
		}
		json.Register(hypCh, json.Handlers{"ad": adHandler}, nil)

		ch := testutils.NewClient(t, nil)
		client, err := NewClient(ch, configFor(hyperbahnHostPort), stubbedSleep())
		assert.NoError(t, err, "hyperbahn NewClient failed")
		defer client.Close()

		assert.Equal(t, errEphemeralPeer, client.Advertise(), "Advertise without Listen should fail")
	})
}

type retryTest struct {
	// channel used to control the response to an 'ad' call.
	respCh chan int
	// reqCh contains the AdRequests sent to the adHandler.
	reqCh chan *AdRequest

	// sleep stub channels.
	sleepArgs  <-chan time.Duration
	sleepBlock chan<- struct{}
	sleepClose func()
	timeSleep  func(time.Duration)

	ch     *tchannel.Channel
	client *Client
	mock   mock.Mock
}

func (r *retryTest) On(event Event) {
	r.mock.Called(event)
}
func (r *retryTest) OnError(err error) {
	r.mock.Called(err)
}

func (r *retryTest) adHandler(ctx json.Context, req *AdRequest) (*AdResponse, error) {
	r.reqCh <- req
	v := <-r.respCh
	if v == 0 {
		return nil, errors.New("failed")
	}
	return &AdResponse{v}, nil
}

func (r *retryTest) setup() {
	r.respCh = make(chan int, 1)
	r.reqCh = make(chan *AdRequest, 1)
	r.sleepArgs, r.sleepBlock, r.sleepClose = testutils.SleepStub(&r.timeSleep)
}

func (r *retryTest) setAdvertiseSuccess() {
	r.respCh <- 1
}

func (r *retryTest) setAdvertiseFailure() {
	r.respCh <- 0
}

func runRetryTest(t *testing.T, f func(r *retryTest)) {
	r := &retryTest{}
	defer testutils.SetTimeout(t, 2*time.Second)()
	r.setup()

	withSetup(t, func(hypCh *tchannel.Channel, hostPort string) {
		json.Register(hypCh, json.Handlers{"ad": r.adHandler}, nil)

		// Advertise failures cause warning log messages.
		opts := testutils.NewOpts().
			SetServiceName("my-client").
			AddLogFilter("Hyperbahn client registration failed", 10)
		serverCh := testutils.NewServer(t, opts)
		defer serverCh.Close()

		var err error
		r.ch = serverCh
		r.client, err = NewClient(serverCh, configFor(hostPort), &ClientOptions{
			Handler:      r,
			FailStrategy: FailStrategyIgnore,
			TimeSleep:    r.timeSleep,
		})
		require.NoError(t, err, "NewClient")
		defer r.client.Close()
		f(r)
		r.mock.AssertExpectations(t)
	})
}

func TestAdvertiseSuccess(t *testing.T) {
	runRetryTest(t, func(r *retryTest) {
		r.mock.On("On", SendAdvertise).Return().
			Times(1 /* initial */ + 10 /* successful retries */)
		r.mock.On("On", Advertised).Return().Once()
		r.setAdvertiseSuccess()
		require.NoError(t, r.client.Advertise())

		// Verify that the arguments passed to 'ad' are correct.
		expectedRequest := &AdRequest{[]service{{Name: "my-client", Cost: 0}}}
		require.Equal(t, expectedRequest, <-r.reqCh)

		// Verify readvertise happen after sleeping for ~advertiseInterval.
		r.mock.On("On", Readvertised).Return().Times(10)
		for i := 0; i < 10; i++ {
			s1 := <-r.sleepArgs
			checkAdvertiseInterval(t, s1)
			r.sleepBlock <- struct{}{}

			r.setAdvertiseSuccess()
			require.Equal(t, expectedRequest, <-r.reqCh)
		}

		// Block till the last advertise completes.
		<-r.sleepArgs
	})
}

func TestMutlipleAdvertise(t *testing.T) {
	runRetryTest(t, func(r *retryTest) {
		r.mock.On("On", SendAdvertise).Return().
			Times(1 /* initial */ + 10 /* successful retries */)
		r.mock.On("On", Advertised).Return().Once()
		r.setAdvertiseSuccess()

		sc2, sc3 := r.ch.GetSubChannel("svc-2"), r.ch.GetSubChannel("svc-3")
		require.NoError(t, r.client.Advertise(sc2, sc3))

		// Verify that the arguments passed to 'ad' are correct.
		expectedRequest := &AdRequest{[]service{
			{Name: "my-client", Cost: 0},
			{Name: "svc-2", Cost: 0},
			{Name: "svc-3", Cost: 0},
		}}
		require.Equal(t, expectedRequest, <-r.reqCh)

		// Verify readvertise happen after sleeping for ~advertiseInterval.
		r.mock.On("On", Readvertised).Return().Times(10)
		for i := 0; i < 10; i++ {
			s1 := <-r.sleepArgs
			checkAdvertiseInterval(t, s1)
			r.sleepBlock <- struct{}{}

			r.setAdvertiseSuccess()
			require.Equal(t, expectedRequest, <-r.reqCh)
		}

		// Block till the last advertise completes.
		<-r.sleepArgs
	})
}

var advertiseErr = json.ErrApplication{"type": "error", "message": "failed"}

func TestRetryTemporaryFailure(t *testing.T) {
	runRetryTest(t, func(r *retryTest) {
		r.mock.On("On", SendAdvertise).Return().
			Times(1 /* initial */ + 3 /* fail */ + 10 /* successful */)
		r.mock.On("On", Advertised).Return().Once()
		r.setAdvertiseSuccess()
		require.NoError(t, r.client.Advertise())
		<-r.reqCh

		s1 := <-r.sleepArgs
		checkAdvertiseInterval(t, s1)

		// When registrations fail, it retries after a short connection and triggers OnError.
		r.mock.On("OnError", ErrAdvertiseFailed{true, advertiseErr}).Return(nil).Times(3)
		for i := 0; i < 3; i++ {
			r.sleepBlock <- struct{}{}
			r.setAdvertiseFailure()

			s1 := <-r.sleepArgs
			<-r.reqCh
			checkRetryInterval(t, s1, i+1 /* retryNum */)
		}

		// If the retry suceeds, then it goes back to normal.
		r.mock.On("On", Readvertised).Return().Times(10)
		// Verify re-registrations continue as usual when it succeeds.
		for i := 0; i < 10; i++ {
			r.sleepBlock <- struct{}{}
			r.setAdvertiseSuccess()

			s1 := <-r.sleepArgs
			<-r.reqCh
			checkAdvertiseInterval(t, s1)
		}
	})
}

func TestRetryFailure(t *testing.T) {
	runRetryTest(t, func(r *retryTest) {
		r.mock.On("On", Advertised).Return().Once()
		r.mock.On("On", SendAdvertise).Return().Times(1 + (maxAdvertiseFailures * 2))

		// Since FailStrategyIgnore will keep retrying forever, we need to
		// signal when we're done testing.
		doneTesting := make(chan struct{}, 1)
		// For all but the last failure, we just need to assert that the
		// OnError handler was called. The callback for the last failure needs
		// to signal that we're done testing and close the Hyperbahn client (so
		// that the Advertise call returns).
		expectationCalled := 0
		r.mock.On("OnError", ErrAdvertiseFailed{true, advertiseErr}).Return(nil).Times(maxAdvertiseFailures * 2).Run(func(_ mock.Arguments) {
			expectationCalled++
			if expectationCalled >= maxAdvertiseFailures*2 {
				close(doneTesting)
				r.client.Close()
			}
		})
		// For the last failure, we assert that the handler was called and
		// signal that the test is done.

		r.setAdvertiseSuccess()
		require.NoError(t, r.client.Advertise())
		<-r.reqCh

		sleptFor := <-r.sleepArgs
		checkAdvertiseInterval(t, sleptFor)

		// Even after maxRegistrationFailures failures to register with
		// Hyperbahn, FailStrategyIgnore should keep retrying.
		for i := 1; i <= maxAdvertiseFailures*2; i++ {
			r.sleepBlock <- struct{}{}
			r.setAdvertiseFailure()
			<-r.reqCh

			sleptFor := <-r.sleepArgs

			// Make sure that we cap backoff at some reasonable duration, even
			// after many retries.
			if i <= maxAdvertiseFailures {
				checkRetryInterval(t, sleptFor, i)
			} else {
				checkRetryInterval(t, sleptFor, maxAdvertiseFailures)
			}
		}

		r.sleepClose()

		// Wait for the handler to be called and the mock expectation to be recorded.
		<-doneTesting
	})
}

func checkAdvertiseInterval(t *testing.T, sleptFor time.Duration) {
	assert.True(t, sleptFor >= advertiseInterval,
		"advertise interval should be > advertiseInterval")
	assert.True(t, sleptFor < advertiseInterval+advertiseFuzzInterval,
		"advertise interval should be < advertiseInterval + advertiseFuzzInterval")
}

func checkRetryInterval(t *testing.T, sleptFor time.Duration, retryNum int) {
	maxRetryInterval := advertiseRetryInterval * time.Duration(1<<uint8(retryNum))
	assert.True(t, sleptFor < maxRetryInterval,
		"retry #%v slept for %v, should sleep for less than %v", retryNum, sleptFor, maxRetryInterval)
}

func configFor(node string) Configuration {
	return Configuration{
		InitialNodes: []string{node},
	}
}

func stubbedSleep() *ClientOptions {
	return &ClientOptions{
		TimeSleep: func(_ time.Duration) {},
	}
}

func withSetup(t *testing.T, f func(ch *tchannel.Channel, hostPort string)) {
	serverCh, err := tchannel.NewChannel(hyperbahnServiceName, nil)
	require.NoError(t, err)
	defer serverCh.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serverCh.Serve(listener)

	f(serverCh, listener.Addr().String())
	serverCh.Close()
}
