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
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"time"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"

	"github.com/uber-go/atomic"
	"golang.org/x/net/context"
)

var (
	hostPort      = flag.String("hostPort", "localhost:12345", "listening socket of the bench server")
	numGoroutines = flag.Int("numGo", 1, "The number of goroutines to spawn")
	numOSThreads  = flag.Int("numThreads", 1, "The number of OS threads to use (sets GOMAXPROCS)")
	setBlockSize  = flag.Int("setBlockSize", 4096, "The size in bytes of the data being set")
	getToSetRatio = flag.Int("getToSetRatio", 1, "The number of Gets to do per Set call")

	// counter tracks the total number of requests completed in the past second.
	counter atomic.Int64
)

func main() {
	flag.Parse()
	runtime.GOMAXPROCS(*numOSThreads)

	// Sets up a listener for pprof.
	go func() {
		log.Println(http.ListenAndServe("localhost:6061", nil))
	}()

	ch, err := tchannel.NewChannel("benchmark-client", nil)
	if err != nil {
		log.Fatalf("NewChannel failed: %v", err)
	}
	for i := 0; i < *numGoroutines; i++ {
		go worker(ch)
	}

	log.Printf("client config: %v workers on %v threads, setBlockSize %v, getToSetRatio %v",
		*numGoroutines, *numOSThreads, *setBlockSize, *getToSetRatio)
	requestCountReporter()
}

func requestCountReporter() {
	for {
		time.Sleep(time.Second)
		cur := counter.Swap(0)
		log.Printf("%v requests", cur)
	}
}

func worker(ch *tchannel.Channel) {
	data := make([]byte, *setBlockSize)
	for {
		if err := setRequest(ch, "key", string(data)); err != nil {
			log.Fatalf("set failed: %v", err)
			continue
		}
		counter.Inc()

		for i := 0; i < *getToSetRatio; i++ {
			_, err := getRequest(ch, "key")
			if err != nil {
				log.Fatalf("get failed: %v", err)
			}
			counter.Inc()
		}
	}
}

func setRequest(ch *tchannel.Channel, key, value string) error {
	ctx, _ := context.WithTimeout(context.Background(), time.Second*10)
	_, _, _, err := raw.Call(ctx, ch, *hostPort, "benchmark", "set", []byte(key), []byte(value))
	return err
}

func getRequest(ch *tchannel.Channel, key string) (string, error) {
	ctx, _ := context.WithTimeout(context.Background(), time.Second)
	_, arg3, _, err := raw.Call(ctx, ch, *hostPort, "benchmark", "get", []byte(key), nil)
	return string(arg3), err
}
