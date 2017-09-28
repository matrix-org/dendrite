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
	"fmt"
	"net"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"

	"github.com/uber-go/atomic"
	"golang.org/x/net/context"
)

// NewServerChannel creates a TChannel that is listening and returns the channel.
// Passed in options may be mutated (for post-verification of state).
func NewServerChannel(opts *ChannelOpts) (*tchannel.Channel, error) {
	opts = opts.Copy()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return nil, fmt.Errorf("could not get listening port from %v: %v", l.Addr().String(), err)
	}

	serviceName := defaultString(opts.ServiceName, DefaultServerName)
	opts.ProcessName = defaultString(opts.ProcessName, serviceName+"-"+port)
	updateOptsLogger(opts)
	ch, err := tchannel.NewChannel(serviceName, &opts.ChannelOptions)
	if err != nil {
		return nil, fmt.Errorf("NewChannel failed: %v", err)
	}

	if err := ch.Serve(l); err != nil {
		return nil, fmt.Errorf("Serve failed: %v", err)
	}

	return ch, nil
}

var totalClients atomic.Uint32

// NewClientChannel creates a TChannel that is not listening.
// Passed in options may be mutated (for post-verification of state).
func NewClientChannel(opts *ChannelOpts) (*tchannel.Channel, error) {
	opts = opts.Copy()

	clientNum := totalClients.Inc()
	serviceName := defaultString(opts.ServiceName, DefaultClientName)
	opts.ProcessName = defaultString(opts.ProcessName, serviceName+"-"+fmt.Sprint(clientNum))
	updateOptsLogger(opts)
	return tchannel.NewChannel(serviceName, &opts.ChannelOptions)
}

type rawFuncHandler struct {
	ch tchannel.Registrar
	f  func(context.Context, *raw.Args) (*raw.Res, error)
}

func (h rawFuncHandler) OnError(ctx context.Context, err error) {
	h.ch.Logger().WithFields(
		tchannel.LogField{Key: "context", Value: ctx},
		tchannel.ErrField(err),
	).Error("simpleHandler OnError.")
}

func (h rawFuncHandler) Handle(ctx context.Context, args *raw.Args) (*raw.Res, error) {
	return h.f(ctx, args)
}

// RegisterFunc registers a function as a handler for the given method name.
func RegisterFunc(ch tchannel.Registrar, name string,
	f func(ctx context.Context, args *raw.Args) (*raw.Res, error)) {

	ch.Register(raw.Wrap(rawFuncHandler{ch, f}), name)
}
