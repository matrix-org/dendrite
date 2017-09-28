// Copyright (c) 2017 Uber Technologies, Inc.

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

// Our glide.yaml lists a set of directories in excludeDirs to avoid packages
// only used in testing from pulling in dependencies that should not affect
// package resolution for clients.
// However, we really want these directories to be part of test imports. Since
// glide does not provide a "testDirs" option, we add dependencies required
// for tests in this _test.go file.

package tchannel_test

import (
	"fmt"
	"testing"

	jcg "github.com/uber/jaeger-client-go"
	// why is this not automatically included from jaeger-client-go?
	// _ "github.com/uber/jaeger-lib/metrics"
)

func TestJaegerDeps(t *testing.T) {
	m := jcg.Metrics{}
	_ = m.SamplerUpdateFailure
	fmt.Println("m", m)
}
