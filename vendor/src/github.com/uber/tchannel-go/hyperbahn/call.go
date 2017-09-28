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

	"github.com/uber/tchannel-go"
)

var errEphemeralPeer = errors.New("cannot advertise on channel that has not called ListenAndServe")

// The following parameters define the request/response for the Hyperbahn 'ad' call.
type service struct {
	Name string `json:"serviceName"`
	Cost int    `json:"cost"`
}

// AdRequest is the Ad request sent to Hyperbahn.
type AdRequest struct {
	Services []service `json:"services"`
}

// AdResponse is the Ad response from Hyperbahn.
type AdResponse struct {
	ConnectionCount int `json:"connectionCount"`
}

func (c *Client) createRequest() *AdRequest {
	req := &AdRequest{
		Services: make([]service, len(c.services)),
	}
	for i, s := range c.services {
		req.Services[i] = service{
			Name: s,
			Cost: 0,
		}
	}
	return req
}

func (c *Client) sendAdvertise() error {
	// Cannot advertise from an ephemeral peer.
	if c.tchan.PeerInfo().IsEphemeralHostPort() {
		return errEphemeralPeer
	}

	retryOpts := &tchannel.RetryOptions{
		RetryOn:           tchannel.RetryIdempotent,
		TimeoutPerAttempt: c.opts.TimeoutPerAttempt,
	}

	ctx, cancel := tchannel.NewContextBuilder(c.opts.Timeout).
		SetRetryOptions(retryOpts).
		// Disable tracing on Hyperbahn advertise messages to avoid cascading failures (see #790).
		DisableTracing().
		Build()
	defer cancel()

	var resp AdResponse
	c.opts.Handler.On(SendAdvertise)
	return c.jsonClient.Call(ctx, "ad", c.createRequest(), &resp)
}
