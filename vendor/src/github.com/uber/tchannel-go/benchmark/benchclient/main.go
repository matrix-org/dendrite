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

// benchclient is used to make requests to a specific server.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/uber/tchannel-go/benchmark"
)

var (
	serviceName = flag.String("service", "bench-server", "The benchmark server's service name")
	timeout     = flag.Duration("timeout", time.Second, "Timeout for each request")
	requestSize = flag.Int("request-size", 10000, "The number of bytes of each request")
	noLibrary   = flag.Bool("no-library", false, "Whether to use the template based library instead of TChannel's client library")
	numClients  = flag.Int("num-clients", 1, "Number of concurrent clients to run in process")
	noDurations = flag.Bool("no-durations", false, "Disable printing of latencies to stdout")
)

func main() {
	flag.Parse()

	opts := []benchmark.Option{
		benchmark.WithServiceName(*serviceName),
		benchmark.WithTimeout(*timeout),
		benchmark.WithRequestSize(*requestSize),
		benchmark.WithNumClients(*numClients),
	}
	if *noLibrary {
		opts = append(opts, benchmark.WithNoLibrary())
	}

	client := benchmark.NewClient(flag.Args(), opts...)
	fmt.Println("bench-client started")

	rdr := bufio.NewScanner(os.Stdin)
	for rdr.Scan() {
		line := rdr.Text()
		parts := strings.Split(line, " ")
		var n int
		var err error
		if len(parts) >= 2 {
			n, err = strconv.Atoi(parts[1])
			if err != nil {
				log.Fatalf("unrecognized number %q: %v", parts[1], err)
			}
		}

		switch cmd := parts[0]; cmd {
		case "warmup":
			if err := client.Warmup(); err != nil {
				log.Fatalf("warmup failed: %v", err)
			}
			fmt.Println("success")
			continue
		case "rcall":
			makeCalls(n, client.RawCall)
		case "tcall":
			makeCalls(n, client.ThriftCall)
		case "quit":
			return
		default:
			log.Fatalf("unrecognized command: %v", line)
		}
	}

	if err := rdr.Err(); err != nil {
		log.Fatalf("Reader failed: %v", err)
	}
}

func makeCalls(n int, f func(n int) ([]time.Duration, error)) {
	durations, err := f(n)
	if err != nil {
		log.Fatalf("Call failed: %v", err)
	}
	if !*noDurations {
		for i, d := range durations {
			if i > 0 {
				fmt.Printf(" ")
			}
			fmt.Printf("%v", d)
		}
	}
	fmt.Println()
}
