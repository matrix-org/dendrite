package thrift_test

import (
	json_encoding "encoding/json"
	"testing"

	"github.com/uber/tchannel-go"
	. "github.com/uber/tchannel-go/testutils/testtracing"
	"github.com/uber/tchannel-go/thrift"
	gen "github.com/uber/tchannel-go/thrift/gen-go/test"

	"golang.org/x/net/context"
)

// ThriftHandler tests tracing over Thrift encoding
type ThriftHandler struct {
	gen.TChanSimpleService // leave nil so calls to unimplemented methods panic.
	TraceHandler

	thriftClient gen.TChanSimpleService
	t            *testing.T
}

func requestFromThrift(req *gen.Data) *TracingRequest {
	r := new(TracingRequest)
	r.ForwardCount = int(req.I3)
	return r
}

func requestToThrift(r *TracingRequest) *gen.Data {
	return &gen.Data{I3: int32(r.ForwardCount)}
}

func responseFromThrift(t *testing.T, res *gen.Data) (*TracingResponse, error) {
	var r TracingResponse
	if err := json_encoding.Unmarshal([]byte(res.S2), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func responseToThrift(t *testing.T, r *TracingResponse) (*gen.Data, error) {
	jsonBytes, err := json_encoding.Marshal(r)
	if err != nil {
		return nil, err
	}
	return &gen.Data{S2: string(jsonBytes)}, nil
}

func (h *ThriftHandler) Call(ctx thrift.Context, arg *gen.Data) (*gen.Data, error) {
	req := requestFromThrift(arg)
	res, err := h.HandleCall(ctx, req,
		func(ctx context.Context, req *TracingRequest) (*TracingResponse, error) {
			tctx := ctx.(thrift.Context)
			res, err := h.thriftClient.Call(tctx, requestToThrift(req))
			if err != nil {
				return nil, err
			}
			return responseFromThrift(h.t, res)
		})
	if err != nil {
		return nil, err
	}
	return responseToThrift(h.t, res)
}

func (h *ThriftHandler) firstCall(ctx context.Context, req *TracingRequest) (*TracingResponse, error) {
	tctx := thrift.Wrap(ctx)
	res, err := h.thriftClient.Call(tctx, requestToThrift(req))
	if err != nil {
		return nil, err
	}
	return responseFromThrift(h.t, res)
}

func TestThriftTracingPropagation(t *testing.T) {
	suite := &PropagationTestSuite{
		Encoding: EncodingInfo{Format: tchannel.Thrift, HeadersSupported: true},
		Register: func(t *testing.T, ch *tchannel.Channel) TracingCall {
			opts := &thrift.ClientOptions{HostPort: ch.PeerInfo().HostPort}
			thriftClient := thrift.NewClient(ch, ch.PeerInfo().ServiceName, opts)
			handler := &ThriftHandler{
				TraceHandler: TraceHandler{Ch: ch},
				t:            t,
				thriftClient: gen.NewTChanSimpleServiceClient(thriftClient),
			}

			// Register Thrift handler
			server := thrift.NewServer(ch)
			server.Register(gen.NewTChanSimpleServiceServer(handler))

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
