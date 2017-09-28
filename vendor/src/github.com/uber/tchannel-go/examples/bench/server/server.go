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
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"sync"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"

	"golang.org/x/net/context"
)

var (
	flagHost      = flag.String("host", "localhost", "The hostname to listen on")
	flagPort      = flag.Int("port", 12345, "The base port to listen on")
	flagInstances = flag.Int("instances", 1, "The number of instances to start")
	flagOSThreads = flag.Int("numThreads", 1, "The number of OS threads to use (sets GOMAXPROCS)")
)

func main() {
	flag.Parse()
	runtime.GOMAXPROCS(*flagOSThreads)

	// Sets up a listener for pprof.
	go func() {
		log.Printf("server pprof endpoint failed: %v", http.ListenAndServe("localhost:6060", nil))
	}()

	for i := 0; i < *flagInstances; i++ {
		if err := setupServer(*flagHost, *flagPort, i); err != nil {
			log.Fatalf("setupServer %v failed: %v", i, err)
		}
	}

	log.Printf("server config: %v threads listening on %v:%v", *flagOSThreads, *flagHost, *flagPort)

	// Listen indefinitely.
	select {}
}

func setupServer(host string, basePort, instanceNum int) error {
	hostPort := fmt.Sprintf("%s:%v", host, basePort+instanceNum)
	ch, err := tchannel.NewChannel("benchmark", &tchannel.ChannelOptions{
		ProcessName: fmt.Sprintf("benchmark-%v", instanceNum),
	})
	if err != nil {
		return fmt.Errorf("NewChannel failed: %v", err)
	}

	handler := raw.Wrap(&kvHandler{vals: make(map[string]string)})
	ch.Register(handler, "ping")
	ch.Register(handler, "get")
	ch.Register(handler, "set")

	if err := ch.ListenAndServe(hostPort); err != nil {
		return fmt.Errorf("ListenAndServe failed: %v", err)
	}

	return nil
}

type kvHandler struct {
	sync.RWMutex
	vals map[string]string
}

func (h *kvHandler) WithLock(write bool, f func()) {
	if write {
		h.Lock()
	} else {
		h.RLock()
	}

	f()

	if write {
		h.Unlock()
	} else {
		h.RUnlock()
	}
}

func (h *kvHandler) Ping(ctx context.Context, args *raw.Args) (*raw.Res, error) {
	return &raw.Res{
		Arg2: []byte("pong"),
	}, nil
}

func (h *kvHandler) Get(ctx context.Context, args *raw.Args) (*raw.Res, error) {
	var arg3 []byte
	h.WithLock(false /* write */, func() {
		arg3 = []byte(h.vals[string(args.Arg2)])
	})

	return &raw.Res{
		Arg2: []byte(fmt.Sprint(len(arg3))),
		Arg3: arg3,
	}, nil
}

func (h *kvHandler) Set(ctx context.Context, args *raw.Args) (*raw.Res, error) {
	h.WithLock(true /* write */, func() {
		h.vals[string(args.Arg2)] = string(args.Arg3)
	})
	return &raw.Res{
		Arg2: []byte("ok"),
		Arg3: []byte("really ok"),
	}, nil
}

func (h *kvHandler) Handle(ctx context.Context, args *raw.Args) (*raw.Res, error) {
	switch args.Method {
	case "ping":
		return h.Ping(ctx, args)
	case "get":
		return h.Get(ctx, args)
	case "put":
		return h.Set(ctx, args)
	default:
		return nil, errors.New("unknown method")
	}
}

func (h *kvHandler) OnError(ctx context.Context, err error) {
	log.Fatalf("OnError %v", err)
}
