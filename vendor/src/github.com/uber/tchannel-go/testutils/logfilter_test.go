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

package testutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/tchannel-go"
)

func TestLogFilterMatches(t *testing.T) {
	msgFilter := LogFilter{
		Filter: "msgFilter",
	}

	fieldsFilter := LogFilter{
		Filter: "msgFilter",
		FieldFilters: map[string]string{
			"f1": "v1",
			"f2": "v2",
		},
	}

	// fields takes a varargs list of strings which it reads as:
	// key, value, key, value...
	fields := func(vals ...string) []tchannel.LogField {
		fs := make([]tchannel.LogField, len(vals)/2)
		for i := 0; i < len(vals); i += 2 {
			fs[i/2] = tchannel.LogField{
				Key:   vals[i],
				Value: vals[i+1],
			}
		}
		return fs
	}

	tests := []struct {
		Filter  LogFilter
		Message string
		Fields  []tchannel.LogField
		Match   bool
	}{
		{
			Filter:  msgFilter,
			Message: "random message",
			Match:   false,
		},
		{
			Filter:  msgFilter,
			Message: "msgFilter",
			Match:   true,
		},
		{
			// Case matters.
			Filter:  msgFilter,
			Message: "msgfilter",
			Match:   false,
		},
		{
			Filter:  msgFilter,
			Message: "abc msgFilterdef",
			Match:   true,
		},
		{
			Filter:  fieldsFilter,
			Message: "random message",
			Fields:  fields("f1", "v1", "f2", "v2"),
			Match:   false,
		},
		{
			Filter:  fieldsFilter,
			Message: "msgFilter",
			Fields:  fields("f1", "v1", "f2", "v2"),
			Match:   true,
		},
		{
			// Field mismatch should not match.
			Filter:  fieldsFilter,
			Message: "msgFilter",
			Fields:  fields("f1", "v0", "f2", "v2"),
			Match:   false,
		},
		{
			// Missing field should not match.
			Filter:  fieldsFilter,
			Message: "msgFilter",
			Fields:  fields("f2", "v2"),
			Match:   false,
		},
		{
			// Extra fields are OK.
			Filter:  fieldsFilter,
			Message: "msgFilter",
			Fields:  fields("f1", "v0", "f2", "v2", "f3", "v3"),
			Match:   false,
		},
	}

	for _, tt := range tests {
		got := tt.Filter.Matches(tt.Message, tt.Fields)
		assert.Equal(t, tt.Match, got, "Filter %+v .Matches(%v, %v) mismatch",
			tt.Filter, tt.Message, tt.Fields)
	}
}
