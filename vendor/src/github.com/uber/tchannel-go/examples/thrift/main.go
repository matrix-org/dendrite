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
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	tchannel "github.com/uber/tchannel-go"
	gen "github.com/uber/tchannel-go/examples/thrift/gen-go/example"
	"github.com/uber/tchannel-go/thrift"
)

func main() {
	var (
		listener net.Listener
		err      error
	)

	if listener, err = setupServer(); err != nil {
		log.Fatalf("setupServer failed: %v", err)
	}

	if err := runClient1("server", listener.Addr()); err != nil {
		log.Fatalf("runClient1 failed: %v", err)
	}

	if err := runClient2("server", listener.Addr()); err != nil {
		log.Fatalf("runClient2 failed: %v", err)
	}

	go listenConsole()

	// Run for 10 seconds, then stop
	time.Sleep(time.Second * 10)
}

func setupServer() (net.Listener, error) {
	tchan, err := tchannel.NewChannel("server", optsFor("server"))
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}

	server := thrift.NewServer(tchan)
	server.Register(gen.NewTChanFirstServer(&firstHandler{}))
	server.Register(gen.NewTChanSecondServer(&secondHandler{}))

	// Serve will set the local peer info, and start accepting sockets in a separate goroutine.
	tchan.Serve(listener)
	return listener, nil
}

func runClient1(hyperbahnService string, addr net.Addr) error {
	tchan, err := tchannel.NewChannel("client1", optsFor("client1"))
	if err != nil {
		return err
	}
	tchan.Peers().Add(addr.String())
	tclient := thrift.NewClient(tchan, hyperbahnService, nil)
	client := gen.NewTChanFirstClient(tclient)

	go func() {
		for {
			ctx, cancel := thrift.NewContext(time.Second)
			res, err := client.Echo(ctx, "Hi")
			log.Println("Echo(Hi) = ", res, ", err: ", err)
			log.Println("AppError() = ", client.AppError(ctx))
			log.Println("BaseCall() = ", client.BaseCall(ctx))
			cancel()
			time.Sleep(100 * time.Millisecond)
		}
	}()
	return nil
}

func runClient2(hyperbahnService string, addr net.Addr) error {
	tchan, err := tchannel.NewChannel("client2", optsFor("client2"))
	if err != nil {
		return err
	}
	tchan.Peers().Add(addr.String())
	tclient := thrift.NewClient(tchan, hyperbahnService, nil)
	client := gen.NewTChanSecondClient(tclient)

	go func() {
		for {
			ctx, cancel := thrift.NewContext(time.Second)
			client.Test(ctx)
			cancel()
			time.Sleep(100 * time.Millisecond)
		}
	}()
	return nil
}

func listenConsole() {
	rdr := bufio.NewReader(os.Stdin)
	for {
		line, _ := rdr.ReadString('\n')
		switch strings.TrimSpace(line) {
		case "s":
			printStack()
		default:
			fmt.Println("Unrecognized command:", line)
		}
	}
}

func printStack() {
	buf := make([]byte, 10000)
	runtime.Stack(buf, true /* all */)
	fmt.Println("Stack:\n", string(buf))
}

type firstHandler struct{}

func (h *firstHandler) Healthcheck(ctx thrift.Context) (*gen.HealthCheckRes, error) {
	log.Printf("first: HealthCheck()\n")
	return &gen.HealthCheckRes{
		Healthy: true,
		Msg:     "OK"}, nil
}

func (h *firstHandler) BaseCall(ctx thrift.Context) error {
	log.Printf("first: BaseCall()\n")
	return nil
}

func (h *firstHandler) Echo(ctx thrift.Context, msg string) (r string, err error) {
	log.Printf("first: Echo(%v)\n", msg)
	return msg, nil
}

func (h *firstHandler) AppError(ctx thrift.Context) error {
	log.Printf("first: AppError()\n")
	return errors.New("app error")
}

func (h *firstHandler) OneWay(ctx thrift.Context) error {
	log.Printf("first: OneWay()\n")
	return errors.New("OneWay error...won't be seen by client")
}

type secondHandler struct{}

func (h *secondHandler) Test(ctx thrift.Context) error {
	log.Println("secondHandler: Test()")
	return nil
}

func optsFor(processName string) *tchannel.ChannelOptions {
	return &tchannel.ChannelOptions{
		ProcessName: processName,
		Logger:      tchannel.NewLevelLogger(tchannel.SimpleLogger, tchannel.LogLevelWarn),
	}
}
