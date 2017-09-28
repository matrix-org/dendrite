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

package raw

import (
	"golang.org/x/net/context"

	"github.com/uber/tchannel-go"
)

// Handler is the interface for a raw handler.
type Handler interface {
	// Handle is called on incoming calls, and contains all the arguments.
	// If an error is returned, it will set ApplicationError Arg3 will be the error string.
	Handle(ctx context.Context, args *Args) (*Res, error)
	OnError(ctx context.Context, err error)
}

// Args parses the arguments from an incoming call req.
type Args struct {
	Caller string
	Format tchannel.Format
	Method string
	Arg2   []byte
	Arg3   []byte
}

// Res represents the response to an incoming call req.
type Res struct {
	SystemErr error
	// IsErr is used to set an application error on the underlying call res.
	IsErr bool
	Arg2  []byte
	Arg3  []byte
}

// ReadArgs reads the *Args from the given call.
func ReadArgs(call *tchannel.InboundCall) (*Args, error) {
	var args Args
	args.Caller = call.CallerName()
	args.Format = call.Format()
	args.Method = string(call.Method())
	if err := tchannel.NewArgReader(call.Arg2Reader()).Read(&args.Arg2); err != nil {
		return nil, err
	}
	if err := tchannel.NewArgReader(call.Arg3Reader()).Read(&args.Arg3); err != nil {
		return nil, err
	}
	return &args, nil
}

// WriteResponse writes the given Res to the InboundCallResponse.
func WriteResponse(response *tchannel.InboundCallResponse, resp *Res) error {
	if resp.SystemErr != nil {
		return response.SendSystemError(resp.SystemErr)
	}
	if resp.IsErr {
		if err := response.SetApplicationError(); err != nil {
			return err
		}
	}
	if err := tchannel.NewArgWriter(response.Arg2Writer()).Write(resp.Arg2); err != nil {
		return err
	}
	return tchannel.NewArgWriter(response.Arg3Writer()).Write(resp.Arg3)
}

// Wrap wraps a Handler as a tchannel.Handler that can be passed to tchannel.Register.
func Wrap(handler Handler) tchannel.Handler {
	return tchannel.HandlerFunc(func(ctx context.Context, call *tchannel.InboundCall) {
		args, err := ReadArgs(call)
		if err != nil {
			handler.OnError(ctx, err)
			return
		}

		resp, err := handler.Handle(ctx, args)
		response := call.Response()
		if err != nil {
			resp = &Res{
				SystemErr: err,
			}
		}
		if err := WriteResponse(response, resp); err != nil {
			handler.OnError(ctx, err)
		}
	})
}
