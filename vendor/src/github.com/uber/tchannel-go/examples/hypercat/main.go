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
	"io"
	"log"
	"net"
	"os"
	"os/exec"

	"github.com/jessevdk/go-flags"
	"github.com/uber/tchannel-go"
	"golang.org/x/net/context"
)

var options = struct {
	ServiceName string `short:"s" long:"service" required:"true" description:"The TChannel/Hyperbahn service name"`

	// MethodName can be specified multiple times to listen on multiple methods.
	MethodName []string `short:"o" long:"method" required:"true" description:"The method name to handle"`

	// HostPort can just be :port or port, in which case host defaults to tchannel's ListenIP.
	HostPort string `short:"l" long:"hostPort" default:":0" description:"The port or host:port to listen on"`

	MaxConcurrency int `short:"m" long:"maxSpawn" default:"1" description:"The maximum number concurrent processes"`

	Cmd struct {
		Command string   `long:"command" description:"The command to execute" positional-arg-name:"command"`
		Args    []string `long:"args" description:"The arguments to pass to the command" positional-arg-name:"args"`
	} `positional-args:"yes" required:"yes"`
}{}

var running chan struct{}

func parseArgs() {
	var err error
	if _, err = flags.Parse(&options); err != nil {
		os.Exit(-1)
	}

	// Convert host port to a real host port.
	host, port, err := net.SplitHostPort(options.HostPort)
	if err != nil {
		port = options.HostPort
	}
	if host == "" {
		hostIP, err := tchannel.ListenIP()
		if err != nil {
			log.Printf("could not get ListenIP: %v, defaulting to 127.0.0.1", err)
			host = "127.0.0.1"
		} else {
			host = hostIP.String()
		}
	}
	options.HostPort = host + ":" + port

	running = make(chan struct{}, options.MaxConcurrency)
}

func main() {
	parseArgs()

	ch, err := tchannel.NewChannel(options.ServiceName, nil)
	if err != nil {
		log.Fatalf("NewChannel failed: %v", err)
	}

	for _, op := range options.MethodName {
		ch.Register(tchannel.HandlerFunc(handler), op)
	}

	if err := ch.ListenAndServe(options.HostPort); err != nil {
		log.Fatalf("ListenAndServe failed: %v", err)
	}

	peerInfo := ch.PeerInfo()
	log.Printf("listening for %v:%v on %v", peerInfo.ServiceName, options.MethodName, peerInfo.HostPort)
	select {}
}

func onError(msg string, args ...interface{}) {
	log.Fatalf(msg, args...)
}

func handler(ctx context.Context, call *tchannel.InboundCall) {
	running <- struct{}{}
	defer func() { <-running }()

	var arg2 []byte
	if err := tchannel.NewArgReader(call.Arg2Reader()).Read(&arg2); err != nil {
		log.Fatalf("Arg2Reader failed: %v", err)
	}

	arg3Reader, err := call.Arg3Reader()
	if err != nil {
		log.Fatalf("Arg3Reader failed: %v", err)
	}

	response := call.Response()
	if err := tchannel.NewArgWriter(response.Arg2Writer()).Write(nil); err != nil {
		log.Fatalf("Arg2Writer failed: %v", err)
	}

	arg3Writer, err := response.Arg3Writer()
	if err != nil {
		log.Fatalf("Arg3Writer failed: %v", err)
	}

	if err := spawnProcess(arg3Reader, arg3Writer); err != nil {
		log.Fatalf("spawnProcess failed: %v", err)
	}

	if err := arg3Reader.Close(); err != nil {
		log.Fatalf("Arg3Reader.Close failed: %v", err)
	}
	if err := arg3Writer.Close(); err != nil {
		log.Fatalf("Arg3Writer.Close failed: %v", err)
	}
}

func spawnProcess(reader io.Reader, writer io.Writer) error {
	cmd := exec.Command(options.Cmd.Command, options.Cmd.Args...)
	cmd.Stdin = reader
	cmd.Stdout = writer
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
