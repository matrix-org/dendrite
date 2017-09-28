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
	"fmt"
	"strconv"
	"strings"
	"time"
)

// externalClient represents a benchmark client running out-of-process.
type externalClient struct {
	*externalCmd
	opts *options
}

func newExternalClient(hosts []string, opts *options) Client {
	benchArgs := []string{
		"--service", opts.svcName,
		"--timeout", opts.timeout.String(),
		"--request-size", strconv.Itoa(opts.reqSize),
		"--num-clients", strconv.Itoa(opts.numClients),
	}
	if opts.noDurations {
		benchArgs = append(benchArgs, "--no-durations")
	}
	if opts.noLibrary {
		benchArgs = append(benchArgs, "--no-library")
	}
	benchArgs = append(benchArgs, hosts...)

	cmd, initial := newExternalCmd("benchclient/main.go", benchArgs)
	if !strings.Contains(initial, "started") {
		panic("bench-client did not start, got: " + initial)
	}

	return &externalClient{cmd, opts}
}

func (c *externalClient) Warmup() error {
	out, err := c.writeAndRead("warmup")
	if err != nil {
		return err
	}
	if out != "success" {
		return fmt.Errorf("warmup failed: %v", out)
	}
	return nil
}

func (c *externalClient) callAndParse(cmd string) ([]time.Duration, error) {
	out, err := c.writeAndRead(cmd)
	if err != nil {
		return nil, err
	}

	if out == "" {
		return nil, nil
	}

	durationStrs := strings.Split(out, " ")
	durations := make([]time.Duration, len(durationStrs))
	for i, s := range durationStrs {
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("calls failed: %v", out)
		}

		durations[i] = d
	}

	return durations, nil
}

func (c *externalClient) RawCall(n int) ([]time.Duration, error) {
	return c.callAndParse(fmt.Sprintf("rcall %v", n))
}

func (c *externalClient) ThriftCall(n int) ([]time.Duration, error) {
	return c.callAndParse(fmt.Sprintf("tcall %v", n))
}
