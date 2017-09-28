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
	"fmt"
	"math/rand"
	"time"

	"github.com/uber/tchannel-go"
)

const (
	// maxAdvertiseFailures is the number of consecutive advertise failures after
	// which we give up and trigger an OnError event.
	maxAdvertiseFailures = 5
	// advertiseInterval is the base time interval between advertisements.
	advertiseInterval = 50 * time.Second
	// advertiseFuzzInterval is the maximum fuzz period to add to advertiseInterval.
	advertiseFuzzInterval = 20 * time.Second
	// advertiseRetryInterval is the unfuzzed base duration to wait before retry on the first
	// advertise failure. Successive retries will use 2 * previous base duration.
	advertiseRetryInterval = 1 * time.Second
)

// ErrAdvertiseFailed is triggered when advertise fails.
type ErrAdvertiseFailed struct {
	// WillRetry is set to true if advertise will be retried.
	WillRetry bool
	// Cause is the underlying error returned from the advertise call.
	Cause error
}

func (e ErrAdvertiseFailed) Error() string {
	return fmt.Sprintf("advertise failed, retry: %v, cause: %v", e.WillRetry, e.Cause)
}

// fuzzInterval returns a fuzzed version of the interval based on FullJitter as described here:
// http://www.awsarchitectureblog.com/2015/03/backoff.html
func fuzzInterval(interval time.Duration) time.Duration {
	return time.Duration(rand.Int63n(int64(interval)))
}

// fuzzedAdvertiseInterval returns the time to sleep between successful advertisements.
func (c *Client) fuzzedAdvertiseInterval() time.Duration {
	return advertiseInterval + fuzzInterval(advertiseFuzzInterval)
}

// logFailedRegistrationRetry logs either a warning or info depending on the number of
// consecutiveFailures. If consecutiveFailures > maxAdvertiseFailures, then we log a warning.
func (c *Client) logFailedRegistrationRetry(errLogger tchannel.Logger, consecutiveFailures uint) {
	logFn := errLogger.Info
	if consecutiveFailures > maxAdvertiseFailures {
		logFn = errLogger.Warn
	}

	logFn("Hyperbahn client registration failed, will retry.")
}

// advertiseLoop readvertises the service approximately every minute (with some fuzzing).
func (c *Client) advertiseLoop() {
	sleepFor := c.fuzzedAdvertiseInterval()
	consecutiveFailures := uint(0)

	for {
		c.sleep(sleepFor)
		if c.IsClosed() {
			c.tchan.Logger().Infof("Hyperbahn client closed")
			return
		}

		if err := c.sendAdvertise(); err != nil {
			consecutiveFailures++
			errLogger := c.tchan.Logger().WithFields(tchannel.ErrField(err))
			if consecutiveFailures >= maxAdvertiseFailures && c.opts.FailStrategy == FailStrategyFatal {
				c.opts.Handler.OnError(ErrAdvertiseFailed{Cause: err, WillRetry: false})
				errLogger.Fatal("Hyperbahn client registration failed.")
			}

			c.logFailedRegistrationRetry(errLogger, consecutiveFailures)
			c.opts.Handler.OnError(ErrAdvertiseFailed{Cause: err, WillRetry: true})

			// Even after many failures, cap backoff.
			if consecutiveFailures < maxAdvertiseFailures {
				sleepFor = fuzzInterval(advertiseRetryInterval * time.Duration(1<<consecutiveFailures))
			}
		} else {
			c.opts.Handler.On(Readvertised)
			sleepFor = c.fuzzedAdvertiseInterval()
			consecutiveFailures = 0
		}
	}
}

// initialAdvertise will do the initial Advertise call to Hyperbahn with additional
// retries on top of the built-in TChannel retries. It will use exponential backoff
// between each of the call attempts.
func (c *Client) initialAdvertise() error {
	var err error
	for attempt := uint(0); attempt < maxAdvertiseFailures; attempt++ {
		err = c.sendAdvertise()
		if err == nil || err == errEphemeralPeer {
			break
		}

		c.tchan.Logger().WithFields(tchannel.ErrField(err)).Info(
			"Hyperbahn client initial registration failure, will retry")

		// Back off for a while.
		sleepFor := fuzzInterval(advertiseRetryInterval * time.Duration(1<<attempt))
		c.sleep(sleepFor)
	}
	return err
}

func (c *Client) sleep(d time.Duration) {
	c.opts.TimeSleep(d)
}
