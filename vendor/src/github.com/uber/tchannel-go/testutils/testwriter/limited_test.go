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

package testwriter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLimitedWriter(t *testing.T) {
	tests := []struct {
		limit      int
		writeBytes []byte
		wantErr    error
		wantBytes  int
	}{
		{
			limit:      1,
			writeBytes: []byte{1},
			wantBytes:  1,
		},
		{
			limit:      1,
			writeBytes: []byte{1, 2},
			wantErr:    ErrOutOfSpace,
			wantBytes:  1,
		},
		{
			limit:      0,
			writeBytes: nil,
			wantBytes:  0,
		},
		{
			limit:      5,
			writeBytes: []byte{1, 2, 3, 4, 5, 6},
			wantErr:    ErrOutOfSpace,
			wantBytes:  5,
		},
	}

	for _, tt := range tests {
		writer := Limited(tt.limit)
		n, err := writer.Write(tt.writeBytes)
		if tt.wantErr != nil {
			assert.Equal(t, tt.wantErr, err, "Write %v to Limited(%v) should fail", tt.writeBytes, tt.limit)
		} else {
			assert.NoError(t, err, "Write %v to Limited(%v) should not fail", tt.writeBytes, tt.limit)
		}
		assert.Equal(t, tt.wantBytes, n, "Unexpected number of bytes written to Limited(%v)", tt.limit)

		n, err = writer.Write([]byte{2})
		assert.Equal(t, ErrOutOfSpace, err, "Write should be out of space")
		assert.Equal(t, 0, n, "Write should not write any bytes when it is out of space")
	}
}

func TestLimitedWriter2(t *testing.T) {
	writer := Limited(1)
	n, err := writer.Write([]byte{1, 2})
	assert.Equal(t, ErrOutOfSpace, err, "Write should fail")
	assert.Equal(t, 1, n, "Write should only write one byte")

	n, err = writer.Write([]byte{2})
	assert.Equal(t, ErrOutOfSpace, err, "Write should be out of space")
	assert.Equal(t, 0, n, "Write should not write any bytes when it is out of space")
}
