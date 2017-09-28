package json_test

import (
	"testing"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/json"

	. "github.com/uber/tchannel-go/testutils/testtracing"
	"golang.org/x/net/context"
)

// JSONHandler tests tracing over JSON encoding
type JSONHandler struct {
	TraceHandler
	t *testing.T
}

func (h *JSONHandler) firstCall(ctx context.Context, req *TracingRequest) (*TracingResponse, error) {
	jctx := json.Wrap(ctx)
	response := new(TracingResponse)
	peer := h.Ch.Peers().GetOrAdd(h.Ch.PeerInfo().HostPort)
	if err := json.CallPeer(jctx, peer, h.Ch.PeerInfo().ServiceName, "call", req, response); err != nil {
		return nil, err
	}
	return response, nil
}

func (h *JSONHandler) callJSON(ctx json.Context, req *TracingRequest) (*TracingResponse, error) {
	return h.HandleCall(ctx, req,
		func(ctx context.Context, req *TracingRequest) (*TracingResponse, error) {
			jctx := ctx.(json.Context)
			peer := h.Ch.Peers().GetOrAdd(h.Ch.PeerInfo().HostPort)
			childResp := new(TracingResponse)
			if err := json.CallPeer(jctx, peer, h.Ch.PeerInfo().ServiceName, "call", req, childResp); err != nil {
				return nil, err
			}
			return childResp, nil
		})
}

func (h *JSONHandler) onError(ctx context.Context, err error) { h.t.Errorf("onError %v", err) }

func TestJSONTracingPropagation(t *testing.T) {
	suite := &PropagationTestSuite{
		Encoding: EncodingInfo{Format: tchannel.JSON, HeadersSupported: true},
		Register: func(t *testing.T, ch *tchannel.Channel) TracingCall {
			handler := &JSONHandler{TraceHandler: TraceHandler{Ch: ch}, t: t}
			json.Register(ch, json.Handlers{"call": handler.callJSON}, handler.onError)
			return handler.firstCall
		},
		TestCases: map[TracerType][]PropagationTestCase{
			Noop: {
				{ForwardCount: 2, TracingDisabled: true, ExpectedBaggage: "", ExpectedSpanCount: 0},
				{ForwardCount: 2, TracingDisabled: false, ExpectedBaggage: "", ExpectedSpanCount: 0},
			},
			Mock: {
				{ForwardCount: 2, TracingDisabled: true, ExpectedBaggage: BaggageValue, ExpectedSpanCount: 0},
				{ForwardCount: 2, TracingDisabled: false, ExpectedBaggage: BaggageValue, ExpectedSpanCount: 6},
			},
			Jaeger: {
				{ForwardCount: 2, TracingDisabled: true, ExpectedBaggage: BaggageValue, ExpectedSpanCount: 0},
				{ForwardCount: 2, TracingDisabled: false, ExpectedBaggage: BaggageValue, ExpectedSpanCount: 6},
			},
		},
	}
	suite.Run(t)
}
