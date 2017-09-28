// Copyright (c) 2016 Uber Technologies, Inc.
//
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

package trace

import (
	"encoding/json"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/crossdock/log"
	"github.com/uber/tchannel-go/thrift"
	gen "github.com/uber/tchannel-go/thrift/gen-go/test"

	"golang.org/x/net/context"
)

func (b *Behavior) registerThrift(ch *tchannel.Channel) {
	handler := &thriftHandler{b: b, ch: ch}
	server := thrift.NewServer(ch)
	server.Register(gen.NewTChanSimpleServiceServer(handler))
	b.thriftCall = handler.callDownstream
}

type thriftHandler struct {
	gen.TChanSimpleService // leave nil so calls to unimplemented methods panic.

	ch *tchannel.Channel
	b  *Behavior
}

func (h *thriftHandler) Call(ctx thrift.Context, arg *gen.Data) (*gen.Data, error) {
	req, err := requestFromThrift(arg)
	if err != nil {
		return nil, err
	}
	res, err := h.b.prepareResponse(ctx, nil, req.Downstream)
	if err != nil {
		return nil, err
	}
	return responseToThrift(res)
}

func (h *thriftHandler) callDownstream(ctx context.Context, target *Downstream) (*Response, error) {
	req := &Request{
		ServerRole: target.ServerRole,
		Downstream: target.Downstream,
	}
	opts := &thrift.ClientOptions{HostPort: target.HostPort}
	thriftClient := thrift.NewClient(h.ch, target.ServiceName, opts)
	serviceClient := gen.NewTChanSimpleServiceClient(thriftClient)
	tReq, err := requestToThrift(req)
	if err != nil {
		return nil, err
	}

	log.Printf("Calling Thrift service %s (%s)", target.ServiceName, target.HostPort)
	tctx := thrift.Wrap(ctx)
	res, err := serviceClient.Call(tctx, tReq)
	if err != nil {
		return nil, err
	}
	return responseFromThrift(res)
}

func requestFromThrift(req *gen.Data) (*Request, error) {
	var r Request
	if err := json.Unmarshal([]byte(req.S2), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func requestToThrift(r *Request) (*gen.Data, error) {
	jsonBytes, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return &gen.Data{S2: string(jsonBytes)}, nil
}

func responseFromThrift(res *gen.Data) (*Response, error) {
	var r Response
	if err := json.Unmarshal([]byte(res.S2), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func responseToThrift(r *Response) (*gen.Data, error) {
	jsonBytes, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return &gen.Data{S2: string(jsonBytes)}, nil
}
