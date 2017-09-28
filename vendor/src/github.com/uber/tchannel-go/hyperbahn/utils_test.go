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

package hyperbahn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntToIP4(t *testing.T) {
	tests := []struct {
		ip       uint32
		expected string
	}{
		{
			ip:       0,
			expected: "0.0.0.0",
		},
		{
			ip:       0x01010101,
			expected: "1.1.1.1",
		},
		{
			ip:       0x01030507,
			expected: "1.3.5.7",
		},
		{
			ip:       0xFFFFFFFF,
			expected: "255.255.255.255",
		},
	}

	for _, tt := range tests {
		got := intToIP4(tt.ip).String()
		assert.Equal(t, tt.expected, got, "IP %v not converted correctly", tt.ip)
	}
}
