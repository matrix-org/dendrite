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

package crossdock

import (
	"testing"

	"github.com/crossdock/crossdock-go/assert"
)

func TestCombinations(t *testing.T) {
	tests := []struct {
		axes map[string][]string
		want []map[string]string
	}{
		{
			axes: nil,
			want: nil,
		},
		{
			axes: map[string][]string{},
			want: nil,
		},
		{
			axes: map[string][]string{
				"x": {"1", "2"},
			},
			want: []map[string]string{
				{"x": "1"},
				{"x": "2"},
			},
		},
		{
			axes: map[string][]string{
				"x": {"1", "2"},
				"y": {"3", "4"},
			},
			want: []map[string]string{
				{"x": "1", "y": "3"},
				{"x": "2", "y": "3"},
				{"x": "1", "y": "4"},
				{"x": "2", "y": "4"},
			},
		},
		{
			axes: map[string][]string{
				"x": {"1", "2"},
				"y": {"3", "4"},
				"z": {"5", "6"},
			},
			want: []map[string]string{
				{"x": "1", "y": "3", "z": "5"},
				{"x": "2", "y": "3", "z": "5"},
				{"x": "1", "y": "4", "z": "5"},
				{"x": "2", "y": "4", "z": "5"},
				{"x": "1", "y": "3", "z": "6"},
				{"x": "2", "y": "3", "z": "6"},
				{"x": "1", "y": "4", "z": "6"},
				{"x": "2", "y": "4", "z": "6"},
			},
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, Combinations(tt.axes))
	}
}
