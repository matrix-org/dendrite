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

package benchmark

import (
	"io"
	"io/ioutil"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func echoServer(tb testing.TB) net.Listener {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(tb, err, "Listen failed")

	go func() {
		conn, err := ln.Accept()
		require.NoError(tb, err, "Accept failed")

		// Echo the connection back to itself.
		io.Copy(conn, conn)
	}()

	return ln
}

func benchmarkClient(b *testing.B, dst string, reqSize int) {
	req := getRequestBytes(reqSize)
	totalExpected := b.N * reqSize

	conn, err := net.Dial("tcp", dst)
	require.NoError(b, err, "Failed to connect to destination")
	defer conn.Close()

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		n, err := io.CopyN(ioutil.Discard, conn, int64(totalExpected))
		assert.NoError(b, err, "Expected %v response bytes, got %v", totalExpected, n)
	}()

	b.SetBytes(int64(reqSize))
	for i := 0; i < b.N; i++ {
		_, err := conn.Write(req)
		require.NoError(b, err, "Write failed")
	}

	<-readerDone
}

func benchmarkTCPDirect(b *testing.B, reqSize int) {
	ln := echoServer(b)
	benchmarkClient(b, ln.Addr().String(), reqSize)
}

func BenchmarkTCPDirect100Bytes(b *testing.B) {
	benchmarkTCPDirect(b, 100)
}

func BenchmarkTCPDirect1k(b *testing.B) {
	benchmarkTCPDirect(b, 1024)
}

func BenchmarkTCPDirect4k(b *testing.B) {
	benchmarkTCPDirect(b, 4*1024)
}

func benchmarkTCPRelay(b *testing.B, reqSize int) {
	ln := echoServer(b)

	relay, err := NewTCPRawRelay([]string{ln.Addr().String()})
	require.NoError(b, err, "Relay failed")
	defer relay.Close()

	benchmarkClient(b, relay.HostPort(), reqSize)
}

func BenchmarkTCPRelay100Bytes(b *testing.B) {
	benchmarkTCPRelay(b, 100)
}

func BenchmarkTCPRelay1kBytes(b *testing.B) {
	benchmarkTCPRelay(b, 1024)
}

func BenchmarkTCPRelay4k(b *testing.B) {
	benchmarkTCPRelay(b, 4*1024)
}
