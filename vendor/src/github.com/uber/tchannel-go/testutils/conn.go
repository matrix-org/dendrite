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
	"net"
	"testing"
)

// GetClosedHostPort will return a host:port that will refuse connections.
func GetClosedHostPort(t testing.TB) string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
		return ""
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("listener.Close failed")
		return ""
	}

	return listener.Addr().String()
}

// GetAcceptCloseHostPort returns a host:port that will accept a connection then
// immediately close it. The returned function can be used to stop the listener.
func GetAcceptCloseHostPort(t testing.TB) (string, func()) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
		return "", nil
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			conn.Close()
		}
	}()

	return listener.Addr().String(), func() {
		if err := listener.Close(); err != nil {
			t.Fatalf("listener.Close failed")
		}
	}
}
