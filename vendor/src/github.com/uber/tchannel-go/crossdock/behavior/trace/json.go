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
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/crossdock/log"
	"github.com/uber/tchannel-go/json"

	"golang.org/x/net/context"
)

const jsonEndpoint = "trace"

func (b *Behavior) registerJSON(ch *tchannel.Channel) {
	handler := &jsonHandler{b: b, ch: ch}
	json.Register(ch, json.Handlers{jsonEndpoint: handler.handleJSON}, handler.onError)
	b.jsonCall = handler.callDownstream
}

type jsonHandler struct {
	ch *tchannel.Channel
	b  *Behavior
}

func (h *jsonHandler) callDownstream(ctx context.Context, target *Downstream) (*Response, error) {
	req := &Request{
		ServerRole: target.ServerRole,
		Downstream: target.Downstream,
	}
	jctx := json.Wrap(ctx)
	response := new(Response)
	log.Printf("Calling JSON service %s (%s)", target.ServiceName, target.HostPort)
	peer := h.ch.Peers().GetOrAdd(target.HostPort)
	if err := json.CallPeer(jctx, peer, target.ServiceName, jsonEndpoint, req, response); err != nil {
		return nil, err
	}
	return response, nil
}

func (h *jsonHandler) handleJSON(ctx json.Context, req *Request) (*Response, error) {
	return h.b.prepareResponse(ctx, nil, req.Downstream)
}

func (h *jsonHandler) onError(ctx context.Context, err error) {
	panic(err.Error())
}
