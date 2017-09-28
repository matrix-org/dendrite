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

package client

import (
	"net/url"
	"testing"

	"github.com/crossdock/crossdock-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientErrors(t *testing.T) {
	c := &Client{
		ClientHostPort: "127.0.0.1:xxx",
	}
	assert.Error(t, c.Start())
}

func TestClientWithBehavior(t *testing.T) {
	r := struct {
		calledA bool
		calledB bool
	}{}
	c := &Client{
		ClientHostPort: "127.0.0.1:0",
		Behaviors: crossdock.Behaviors{
			"hello": func(t crossdock.T) {
				if t.Param("param") == "a" {
					t.Successf("good")
					r.calledA = true
				}
				if t.Param("param") == "b" {
					t.Skipf("not so good")
					r.calledB = true
				}
			},
		},
	}
	require.NoError(t, c.Start())
	defer c.Close()
	crossdock.Wait(t, c.URL(), 10)

	axes := map[string][]string{
		"param": {"a", "b"},
	}
	for _, entry := range crossdock.Combinations(axes) {
		entryArgs := url.Values{}
		for k, v := range entry {
			entryArgs.Set(k, v)
		}
		// test via real HTTP call
		crossdock.Call(t, c.URL(), "hello", entryArgs)
	}
	assert.True(t, r.calledA)
	assert.True(t, r.calledB)
}
