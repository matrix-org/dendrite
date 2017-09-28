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
	"fmt"

	"github.com/uber/tchannel-go"

	"golang.org/x/net/context"
)

// ErrApplication is an application error which contains the object returned from the other side.
type ErrApplication map[string]interface{}

func (e ErrApplication) Error() string {
	return fmt.Sprintf("JSON call failed: %v", map[string]interface{}(e))
}

// Client is used to make JSON calls to other services.
type Client struct {
	ch            *tchannel.Channel
	targetService string
	hostPort      string
}

// ClientOptions are options used when creating a client.
type ClientOptions struct {
	HostPort string
}

// NewClient returns a json.Client used to make outbound JSON calls.
func NewClient(ch *tchannel.Channel, targetService string, opts *ClientOptions) *Client {
	client := &Client{
		ch:            ch,
		targetService: targetService,
	}
	if opts != nil && opts.HostPort != "" {
		client.hostPort = opts.HostPort
	}
	return client
}

func makeCall(call *tchannel.OutboundCall, headers, arg3In, respHeaders, arg3Out, errorOut interface{}) (bool, string, error) {
	if mapHeaders, ok := headers.(map[string]string); ok {
		headers = tchannel.InjectOutboundSpan(call.Response(), mapHeaders)
	}
	if err := tchannel.NewArgWriter(call.Arg2Writer()).WriteJSON(headers); err != nil {
		return false, "arg2 write failed", err
	}
	if err := tchannel.NewArgWriter(call.Arg3Writer()).WriteJSON(arg3In); err != nil {
		return false, "arg3 write failed", err
	}

	// Call Arg2Reader before checking application error.
	if err := tchannel.NewArgReader(call.Response().Arg2Reader()).ReadJSON(respHeaders); err != nil {
		return false, "arg2 read failed", err
	}

	// If this is an error response, read the response into a map and return a jsonCallErr.
	if call.Response().ApplicationError() {
		if err := tchannel.NewArgReader(call.Response().Arg3Reader()).ReadJSON(errorOut); err != nil {
			return false, "arg3 read error failed", err
		}
		return false, "", nil
	}

	if err := tchannel.NewArgReader(call.Response().Arg3Reader()).ReadJSON(arg3Out); err != nil {
		return false, "arg3 read failed", err
	}

	return true, "", nil
}

func (c *Client) startCall(ctx context.Context, method string, callOptions *tchannel.CallOptions) (*tchannel.OutboundCall, error) {
	if c.hostPort != "" {
		return c.ch.BeginCall(ctx, c.hostPort, c.targetService, method, callOptions)
	}

	return c.ch.GetSubChannel(c.targetService).BeginCall(ctx, method, callOptions)
}

// Call makes a JSON call, with retries.
func (c *Client) Call(ctx Context, method string, arg, resp interface{}) error {
	var (
		headers = ctx.Headers()

		respHeaders map[string]string
		respErr     ErrApplication
		errAt       string
		isOK        bool
	)

	err := c.ch.RunWithRetry(ctx, func(ctx context.Context, rs *tchannel.RequestState) error {
		respHeaders, respErr, isOK = nil, nil, false
		errAt = "connect"

		call, err := c.startCall(ctx, method, &tchannel.CallOptions{
			Format:       tchannel.JSON,
			RequestState: rs,
		})
		if err != nil {
			return err
		}

		isOK, errAt, err = makeCall(call, headers, arg, &respHeaders, resp, &respErr)
		return err
	})
	if err != nil {
		// TODO: Don't lose the error type here.
		return fmt.Errorf("%s: %v", errAt, err)
	}
	if !isOK {
		return respErr
	}

	return nil
}

// TODO(prashantv): Clean up json.Call* interfaces.
func wrapCall(ctx Context, call *tchannel.OutboundCall, method string, arg, resp interface{}) error {
	var respHeaders map[string]string
	var respErr ErrApplication
	isOK, errAt, err := makeCall(call, ctx.Headers(), arg, &respHeaders, resp, &respErr)
	if err != nil {
		return fmt.Errorf("%s: %v", errAt, err)
	}
	if !isOK {
		return respErr
	}

	ctx.SetResponseHeaders(respHeaders)
	return nil
}

// CallPeer makes a JSON call using the given peer.
func CallPeer(ctx Context, peer *tchannel.Peer, serviceName, method string, arg, resp interface{}) error {
	call, err := peer.BeginCall(ctx, serviceName, method, &tchannel.CallOptions{Format: tchannel.JSON})
	if err != nil {
		return err
	}

	return wrapCall(ctx, call, method, arg, resp)
}

// CallSC makes a JSON call using the given subchannel.
func CallSC(ctx Context, sc *tchannel.SubChannel, method string, arg, resp interface{}) error {
	call, err := sc.BeginCall(ctx, method, &tchannel.CallOptions{Format: tchannel.JSON})
	if err != nil {
		return err
	}

	return wrapCall(ctx, call, method, arg, resp)
}
