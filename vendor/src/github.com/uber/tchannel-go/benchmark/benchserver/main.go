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

// benchserver is used to receive requests for benchmarks.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/uber/tchannel-go/benchmark"
)

var (
	serviceName    = flag.String("service", "bench-server", "The benchmark server's service name")
	advertiseHosts = flag.String("advertise-hosts", "", "Comma-separated list of hosts to advertise to")
)

func main() {
	flag.Parse()

	var adHosts []string
	if len(*advertiseHosts) > 0 {
		adHosts = strings.Split(*advertiseHosts, ",")
	}

	server := benchmark.NewServer(
		benchmark.WithServiceName(*serviceName),
		benchmark.WithAdvertiseHosts(adHosts),
	)

	fmt.Println(server.HostPort())

	rdr := bufio.NewReader(os.Stdin)
	for {
		line, err := rdr.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return
			}
			log.Fatalf("stdin read failed: %v", err)
		}

		line = strings.TrimSuffix(line, "\n")
		switch line {
		case "count-raw":
			fmt.Println(server.RawCalls())
		case "count-thrift":
			fmt.Println(server.ThriftCalls())
		case "quit":
			return
		default:
			log.Fatalf("unrecognized command: %v", line)
		}
	}
}
