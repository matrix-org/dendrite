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

package tnet

import (
	"errors"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListenerAcceptAfterClose(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for i := 0; i < 10; i++ {
				runTest(t)
			}
		}()
	}
	wg.Wait()
}

func runTest(t *testing.T) {
	const connectionsBeforeClose = 1

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err, "Listen failed") {
		return

	}
	ln = Wrap(ln)

	addr := ln.Addr().String()
	waitForListener := make(chan error)
	go func() {
		defer close(waitForListener)

		var connCount int
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			connCount++
			if connCount > connectionsBeforeClose {
				waitForListener <- errors.New("got unexpected conn")
				return
			}
			conn.Close()
		}
	}()

	for i := 0; i < connectionsBeforeClose; i++ {
		err := connect(addr)
		if !assert.NoError(t, err, "connect before listener is closed should succeed") {
			return
		}
	}

	ln.Close()
	connect(addr)

	err = <-waitForListener
	assert.NoError(t, err, "got connection after listener was closed")
}

func connect(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err == nil {
		conn.Close()
	}
	return err
}
