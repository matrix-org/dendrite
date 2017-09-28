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
	"strings"
	"sync"
)

// DefaultMetricPrefix is the default mapping for metrics to statsd keys.
// It uses a "tchannel" prefix for all stats.
func DefaultMetricPrefix(name string, tags map[string]string) string {
	return MetricWithPrefix("tchannel.", name, tags)
}

var bufPool = sync.Pool{
	New: func() interface{} { return &bytes.Buffer{} },
}

// MetricWithPrefix is the default mapping for metrics to statsd keys.
func MetricWithPrefix(prefix, name string, tags map[string]string) string {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()

	if prefix != "" {
		buf.WriteString(prefix)
	}
	buf.WriteString(name)

	addKeys := make([]string, 0, 5)
	switch {
	case strings.HasPrefix(name, "outbound"):
		addKeys = append(addKeys, "service", "target-service", "target-endpoint")
		if strings.HasPrefix(name, "outbound.calls.retries") {
			addKeys = append(addKeys, "retry-count")
		}
	case strings.HasPrefix(name, "inbound"):
		addKeys = append(addKeys, "calling-service", "service", "endpoint")
	}

	for _, k := range addKeys {
		buf.WriteByte('.')
		v, ok := tags[k]
		if ok {
			writeClean(buf, v)
		} else {
			buf.WriteString("no-")
			buf.WriteString(k)
		}
	}

	m := buf.String()
	bufPool.Put(buf)
	return m
}

// writeClean writes v, after replacing special characters [{}/\\:\s.] with '-'
func writeClean(buf *bytes.Buffer, v string) {
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch c {
		case '{', '}', '/', '\\', ':', '.', ' ', '\t', '\r', '\n':
			buf.WriteByte('-')
		default:
			buf.WriteByte(c)
		}
	}
}
