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

package http

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/tchannel-go/testutils"
	"github.com/uber/tchannel-go/typed"
)

func TestHeaders(t *testing.T) {
	tests := []http.Header{
		{},
		{
			"K1": []string{"K1V1", "K1V2", "K1V3"},
			"K2": []string{"K2V2", "K2V2"},
		},
	}

	for _, tt := range tests {
		buf := make([]byte, 1000)
		wb := typed.NewWriteBuffer(buf)
		writeHeaders(wb, tt)

		newHeaders := make(http.Header)
		rb := typed.NewReadBuffer(buf)
		readHeaders(rb, newHeaders)
		assert.Equal(t, tt, newHeaders, "Headers mismatch")
	}
}

func TestVarintString(t *testing.T) {
	tests := []string{
		"",
		"short string",
		testutils.RandString(1000),
	}

	for _, tt := range tests {
		buf := make([]byte, 2000)
		wb := typed.NewWriteBuffer(buf)
		writeVarintString(wb, tt)

		rb := typed.NewReadBuffer(buf)
		got := readVarintString(rb)
		assert.Equal(t, tt, got, "Varint string mismatch")
	}
}
