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

package tchannel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetHeaders(t *testing.T) {
	tests := []struct {
		format          Format
		routingDelegate string
		routingKey      string
		expectedHeaders transportHeaders
	}{
		{
			// When no format is specified, Raw should be used by default.
			format:          "",
			expectedHeaders: transportHeaders{ArgScheme: Raw.String()},
		},
		{
			format:          Thrift,
			expectedHeaders: transportHeaders{ArgScheme: Thrift.String()},
		},
		{
			format:          JSON,
			routingDelegate: "xpr",
			expectedHeaders: transportHeaders{
				ArgScheme:       JSON.String(),
				RoutingDelegate: "xpr",
			},
		},
		{
			format:     JSON,
			routingKey: "canary",
			expectedHeaders: transportHeaders{
				ArgScheme:  JSON.String(),
				RoutingKey: "canary",
			},
		},
	}

	for _, tt := range tests {
		callOpts := &CallOptions{
			Format:          tt.format,
			RoutingDelegate: tt.routingDelegate,
			RoutingKey:      tt.routingKey,
		}
		headers := make(transportHeaders)
		callOpts.setHeaders(headers)
		assert.Equal(t, tt.expectedHeaders, headers)
	}
}
