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

import "time"

// BenchmarkDir should be set to the benchmark source directory.
var BenchmarkDir = "./"

// Client is a benchmark client that can be used to call a benchmark server.
type Client interface {
	// Warmup will create connections to all host:ports the client was created with.
	Warmup() error

	// RawCall makes an echo call using raw.
	RawCall(n int) ([]time.Duration, error)

	// ThriftCall makes an echo call using thrift.
	ThriftCall(n int) ([]time.Duration, error)

	// Close closes the benchmark client.
	Close()
}

// inProcClient represents a client that is running in the same process.
// It adds methods to reduce allocations.
type inProcClient interface {
	Client

	// RawCallBuffer will make n raw calls and store the latencies in the specified buffer.
	RawCallBuffer(latencies []time.Duration) error

	// ThriftCallBuffer will make n thrift calls and store the latencies in the specified buffer.
	ThriftCallBuffer(latencies []time.Duration) error
}

// Server is a benchmark server that can receive requests.
type Server interface {
	// HostPort returns the HostPort that the server is listening on.
	HostPort() string

	// Close closes the benchmark server.
	Close()

	// RawCalls returns the number of raw calls the server has received.
	RawCalls() int

	// ThriftCalls returns the number of Thrift calls the server has received.
	ThriftCalls() int
}

// Relay represents a relay for benchmarking.
type Relay interface {
	// HostPort is the host:port that the relay is listening on.
	HostPort() string

	// Close clsoes the relay.
	Close()
}
