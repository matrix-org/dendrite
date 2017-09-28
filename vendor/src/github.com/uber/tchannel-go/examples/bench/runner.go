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
	"net"
	"os"
	"os/exec"
	"time"
)

var (
	flagHostPort         = flag.String("hostPort", "127.0.0.1:12345", "The host:port to run the benchmark on")
	flagServerNumThreads = flag.Int("serverThreads", 1, "The number of OS threads to use for the server")

	flagServerBinary = flag.String("serverBinary", "./build/examples/bench/server", "Server binary location")
	flagClientBinary = flag.String("clientBinary", "./build/examples/bench/client", "Client binary location")

	flagProfileAfter = flag.Duration("profileAfter", 0,
		"Duration to wait before profiling. 0 disables profiling. Process is stopped after the profile.")
	flagProfileSeconds = flag.Int("profileSeconds", 30, "The number of seconds to profile")
	flagProfileStop    = flag.Bool("profileStopProcess", true, "Whether to stop the benchmarks after profiling")
)

func main() {
	flag.Parse()

	server, err := runServer(*flagHostPort, *flagServerNumThreads)
	if err != nil {
		log.Fatalf("Server failed: %v", err)
	}
	defer server.Process.Kill()

	client, err := runClient(flag.Args())
	if err != nil {
		log.Fatalf("Client failed: %v", err)
	}
	defer client.Process.Kill()

	if *flagProfileAfter != 0 {
		go func() {
			time.Sleep(*flagProfileAfter)

			// Profile the server and the client, which have pprof endpoints at :6060, and :6061
			p1, err := dumpProfile("localhost:6060", "server.pb.gz")
			if err != nil {
				log.Printf("Server profile failed: %v", err)
			}
			p2, err := dumpProfile("localhost:6061", "client.pb.gz")
			if err != nil {
				log.Printf("Client profile failed: %v", err)
			}
			if err := p1.Wait(); err != nil {
				log.Printf("Server profile error: %v", err)
			}
			if err := p2.Wait(); err != nil {
				log.Printf("Client profile error: %v", err)
			}

			if !*flagProfileStop {
				return
			}

			server.Process.Signal(os.Interrupt)
			client.Process.Signal(os.Interrupt)

			// After a while, kill the processes if we're still running.
			time.Sleep(300 * time.Millisecond)
			log.Printf("Still waiting for processes to stop, sending kill")
			server.Process.Kill()
			client.Process.Kill()
		}()
	}

	server.Wait()
	client.Wait()
}

func runServer(hostPort string, numThreads int) (*exec.Cmd, error) {
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return nil, err
	}

	return runCmd(*flagServerBinary,
		"--host", host,
		"--port", port,
		"--numThreads", fmt.Sprint(numThreads))
}

func runClient(clientArgs []string) (*exec.Cmd, error) {
	return runCmd(*flagClientBinary, clientArgs...)
}

func dumpProfile(baseHostPort, profileFile string) (*exec.Cmd, error) {
	profileURL := fmt.Sprintf("http://%v/debug/pprof/profile", baseHostPort)
	return runCmd("go", "tool", "pprof", "--proto", "--output="+profileFile,
		fmt.Sprintf("--seconds=%v", *flagProfileSeconds), profileURL)
}

func runCmd(cmdBinary string, args ...string) (*exec.Cmd, error) {
	cmd := exec.Command(cmdBinary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, cmd.Start()
}
