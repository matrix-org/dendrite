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

package thrift_test

import (
	"testing"
	"time"

	// Test is in a separate package to avoid circular dependencies.
	. "github.com/uber/tchannel-go/thrift"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"
	gen "github.com/uber/tchannel-go/thrift/gen-go/test"
	"github.com/uber/tchannel-go/thrift/mocks"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serializeStruct(t *testing.T, s thrift.TStruct) []byte {
	trans := thrift.NewTMemoryBuffer()
	p := thrift.NewTBinaryProtocolTransport(trans)
	require.NoError(t, s.Write(p), "Struct serialization failed")
	return trans.Bytes()
}

func TestInvalidThriftBytes(t *testing.T) {
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	ch := testutils.NewClient(t, nil)
	sCh := testutils.NewServer(t, nil)
	defer sCh.Close()

	svr := NewServer(sCh)
	svr.Register(gen.NewTChanSecondServiceServer(new(mocks.TChanSecondService)))

	tests := []struct {
		name string
		arg3 []byte
	}{
		{
			name: "missing bytes",
			arg3: serializeStruct(t, &gen.SecondServiceEchoArgs{Arg: "Hello world"})[:5],
		},
		{
			name: "wrong struct",
			arg3: serializeStruct(t, &gen.Data{B1: true}),
		},
	}

	for _, tt := range tests {
		sPeer := sCh.PeerInfo()
		call, err := ch.BeginCall(ctx, sPeer.HostPort, sPeer.ServiceName, "SecondService::Echo", &tchannel.CallOptions{
			Format: tchannel.Thrift,
		})
		require.NoError(t, err, "BeginCall failed")
		require.NoError(t, tchannel.NewArgWriter(call.Arg2Writer()).Write([]byte{0, 0}), "Write arg2 failed")

		writer, err := call.Arg3Writer()
		require.NoError(t, err, "Arg3Writer failed")

		_, err = writer.Write(tt.arg3)
		require.NoError(t, err, "Write arg3 failed")
		require.NoError(t, writer.Close(), "Close failed")

		response := call.Response()
		_, _, err = raw.ReadArgsV2(response)
		assert.Error(t, err, "%v: Expected error", tt.name)
		assert.Equal(t, tchannel.ErrCodeBadRequest, tchannel.GetSystemErrorCode(err),
			"%v: Expected bad request, got %v", tt.name, err)
	}
}
