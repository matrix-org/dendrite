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
	"net"
	"strconv"
	"strings"
)

// externalServer represents a benchmark server running out-of-process.
type externalServer struct {
	*externalCmd
	hostPort string
	opts     *options
}

func newExternalServer(opts *options) Server {
	benchArgs := []string{
		"--service", opts.svcName,
	}
	if len(opts.advertiseHosts) > 0 {
		benchArgs = append(benchArgs,
			"--advertise-hosts", strings.Join(opts.advertiseHosts, ","))
	}

	cmd, hostPortStr := newExternalCmd("benchserver/main.go", benchArgs)
	if _, _, err := net.SplitHostPort(hostPortStr); err != nil {
		panic("bench-server did not print host:port on startup: " + err.Error())
	}

	return &externalServer{cmd, hostPortStr, opts}
}

func (s *externalServer) HostPort() string {
	return s.hostPort
}

func (s *externalServer) RawCalls() int {
	return s.writeAndReadInt("count-raw")
}

func (s *externalServer) ThriftCalls() int {
	return s.writeAndReadInt("count-thrift")
}

func (s *externalServer) writeAndReadInt(cmd string) int {
	v, err := s.writeAndRead(cmd)
	if err != nil {
		panic(err)
	}

	vInt, err := strconv.Atoi(v)
	if err != nil {
		panic(err)
	}

	return vInt
}
