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

package main

import (
	"flag"
	"fmt"
	"log"

	"golang.org/x/net/context"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"
)

var (
	flagHost = flag.String("host", "localhost", "The hostname to serve on")
	flagPort = flag.Int("port", 0, "The port to listen on")
)

type rawHandler struct{}

func (rawHandler) Handle(ctx context.Context, args *raw.Args) (*raw.Res, error) {
	return &raw.Res{
		Arg2: args.Arg2,
		Arg3: args.Arg3,
	}, nil
}

func (rawHandler) OnError(ctx context.Context, err error) {
	log.Fatalf("OnError: %v", err)
}

func main() {
	flag.Parse()

	ch, err := tchannel.NewChannel("test_as_raw", nil)
	if err != nil {
		log.Fatalf("NewChannel failed: %v", err)
	}

	handler := raw.Wrap(rawHandler{})
	ch.Register(handler, "echo")
	ch.Register(handler, "streaming_echo")

	hostPort := fmt.Sprintf("%s:%v", *flagHost, *flagPort)
	if err := ch.ListenAndServe(hostPort); err != nil {
		log.Fatalf("ListenAndServe failed: %v", err)
	}

	fmt.Println("listening on", ch.PeerInfo().HostPort)
	select {}
}
