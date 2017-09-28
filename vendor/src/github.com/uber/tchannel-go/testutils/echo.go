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

package testutils

import (
	"testing"
	"time"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

// CallEcho calls the "echo" endpoint from the given src to target.
func CallEcho(src *tchannel.Channel, targetHostPort, targetService string, args *raw.Args) error {
	ctx, cancel := tchannel.NewContext(Timeout(300 * time.Millisecond))
	defer cancel()

	if args == nil {
		args = &raw.Args{}
	}

	_, _, _, err := raw.Call(ctx, src, targetHostPort, targetService, "echo", args.Arg2, args.Arg3)
	return err
}

// AssertEcho calls the "echo" endpoint with random data, and asserts
// that the returned data matches the arguments "echo" was called with.
func AssertEcho(tb testing.TB, src *tchannel.Channel, targetHostPort, targetService string) {
	ctx, cancel := tchannel.NewContext(Timeout(300 * time.Millisecond))
	defer cancel()

	args := &raw.Args{
		Arg2: RandBytes(1000),
		Arg3: RandBytes(1000),
	}

	arg2, arg3, _, err := raw.Call(ctx, src, targetHostPort, targetService, "echo", args.Arg2, args.Arg3)
	if !assert.NoError(tb, err, "Call from %v (%v) to %v (%v) failed", src.ServiceName(), src.PeerInfo().HostPort, targetService, targetHostPort) {
		return
	}

	assert.Equal(tb, args.Arg2, arg2, "Arg2 mismatch")
	assert.Equal(tb, args.Arg3, arg3, "Arg3 mismatch")
}

// RegisterEcho registers an echo endpoint on the given channel. The optional provided
// function is run before the handler returns.
func RegisterEcho(src tchannel.Registrar, f func()) {
	RegisterFunc(src, "echo", func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
		if f != nil {
			f()
		}
		return &raw.Res{Arg2: args.Arg2, Arg3: args.Arg3}, nil
	})
}

// Ping sends a ping from src to target.
func Ping(src, target *tchannel.Channel) error {
	ctx, cancel := tchannel.NewContext(Timeout(100 * time.Millisecond))
	defer cancel()

	return src.Ping(ctx, target.PeerInfo().HostPort)
}
