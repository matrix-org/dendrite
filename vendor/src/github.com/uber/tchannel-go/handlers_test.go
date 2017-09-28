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
	"golang.org/x/net/context"
)

type dummyHandler struct{}

func (dummyHandler) Handle(ctx context.Context, call *InboundCall) {}

func TestHandlers(t *testing.T) {
	const (
		m1 = "m1"
		m2 = "m2"
	)
	var (
		hmap = &handlerMap{}

		h1 = &dummyHandler{}
		h2 = &dummyHandler{}

		m1b = []byte(m1)
		m2b = []byte(m2)
	)

	assert.Nil(t, hmap.find(m1b))
	assert.Nil(t, hmap.find(m2b))

	hmap.register(h1, m1)
	assert.Equal(t, h1, hmap.find(m1b))
	assert.Nil(t, hmap.find(m2b))

	hmap.register(h2, m2)
	assert.Equal(t, h1, hmap.find(m1b))
	assert.Equal(t, h2, hmap.find(m2b))
}
