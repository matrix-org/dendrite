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

package stats

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultMetricPrefix(t *testing.T) {
	outboundTags := map[string]string{
		"service":         "callerS",
		"target-service":  "targetS",
		"target-endpoint": "targetE",
		"retry-count":     "retryN",
	}
	inboundTags := map[string]string{
		"service":         "targetS",
		"endpoint":        "targetE",
		"calling-service": "callerS",
	}

	tests := []struct {
		name     string
		tags     map[string]string
		expected string
	}{
		{
			name:     "outbound.calls.sent",
			tags:     outboundTags,
			expected: "tchannel.outbound.calls.sent.callerS.targetS.targetE",
		},
		{
			name:     "outbound.calls.retries",
			tags:     outboundTags,
			expected: "tchannel.outbound.calls.retries.callerS.targetS.targetE.retryN",
		},
		{
			name:     "inbound.calls.recvd",
			tags:     inboundTags,
			expected: "tchannel.inbound.calls.recvd.callerS.targetS.targetE",
		},
		{
			name:     "inbound.calls.recvd",
			tags:     nil,
			expected: "tchannel.inbound.calls.recvd.no-calling-service.no-service.no-endpoint",
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, DefaultMetricPrefix(tt.name, tt.tags),
			"DefaultMetricPrefix(%q, %v) failed", tt.name, tt.tags)
	}
}

func TestClean(t *testing.T) {
	tests := []struct {
		key      string
		expected string
	}{
		{"metric", "metric"},
		{"met:ric", "met-ric"},
		{"met{}ric", "met--ric"},
		{"\\metric", "-metric"},
		{"/metric", "-metric"},
		{"  met.ric  ", "--met-ric--"},
	}

	for _, tt := range tests {
		buf := &bytes.Buffer{}
		writeClean(buf, tt.key)
		assert.Equal(t, tt.expected, buf.String(), "clean(%q) failed", tt.key)
	}
}

func BenchmarkMetricPrefix(b *testing.B) {
	outboundTags := map[string]string{
		"service":         "callerS",
		"target-service":  "targetS",
		"target-endpoint": "targetE",
		"retry-count":     "retryN",
	}

	for i := 0; i < b.N; i++ {
		MetricWithPrefix("", "outbound.calls.retries", outboundTags)
		DefaultMetricPrefix("outbound.calls.retries", outboundTags)
	}
}
