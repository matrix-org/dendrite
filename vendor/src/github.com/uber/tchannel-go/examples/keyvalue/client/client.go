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
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/examples/keyvalue/gen-go/keyvalue"
	"github.com/uber/tchannel-go/hyperbahn"
	"github.com/uber/tchannel-go/thrift"
)

var curUser = "anonymous"

func printHelp() {
	fmt.Println("Usage:\n get [key]\n set [key] [value]")
	fmt.Println(" user [newUser]\n clearAll")
}

func main() {
	// Create a TChannel.
	ch, err := tchannel.NewChannel("keyvalue-client", nil)
	if err != nil {
		log.Fatalf("Failed to create tchannel: %v", err)
	}

	// Set up Hyperbahn client.
	config := hyperbahn.Configuration{InitialNodes: os.Args[1:]}
	if len(config.InitialNodes) == 0 {
		log.Fatalf("No Autobahn nodes to connect to given")
	}
	hyperbahn.NewClient(ch, config, nil)

	thriftClient := thrift.NewClient(ch, "keyvalue", nil)
	client := keyvalue.NewTChanKeyValueClient(thriftClient)
	adminClient := keyvalue.NewTChanAdminClient(thriftClient)

	// Read commands from the command line and execute them.
	scanner := bufio.NewScanner(os.Stdin)
	printHelp()
	fmt.Printf("> ")
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), " ")
		if parts[0] == "" {
			continue
		}
		switch parts[0] {
		case "help":
			printHelp()
		case "get":
			if len(parts) < 2 {
				printHelp()
				break
			}
			get(client, parts[1])
		case "set":
			if len(parts) < 3 {
				printHelp()
				break
			}
			set(client, parts[1], parts[2])
		case "user":
			if len(parts) < 2 {
				printHelp()
				break
			}
			curUser = parts[1]
		case "clearAll":
			clear(adminClient)
		default:
			log.Printf("Unsupported command %q\n", parts[0])
		}
		fmt.Print("> ")
	}
	scanner.Text()
}

func get(client keyvalue.TChanKeyValue, key string) {
	ctx, cancel := createContext()
	defer cancel()

	val, err := client.Get(ctx, key)
	if err != nil {
		switch err := err.(type) {
		case *keyvalue.InvalidKey:
			log.Printf("Get %v failed: invalid key", key)
		case *keyvalue.KeyNotFound:
			log.Printf("Get %v failed: key not found", key)
		default:
			log.Printf("Get %v failed unexpectedly: %v", key, err)
		}
		return
	}

	log.Printf("Get %v: %v", key, val)
}

func set(client keyvalue.TChanKeyValue, key, value string) {
	ctx, cancel := createContext()
	defer cancel()

	if err := client.Set(ctx, key, value); err != nil {
		switch err := err.(type) {
		case *keyvalue.InvalidKey:
			log.Printf("Set %v failed: invalid key", key)
		default:
			log.Printf("Set %v:%v failed unexpectedly: %#v", key, value, err)
		}
		return
	}

	log.Printf("Set %v:%v succeeded with headers: %v", key, value, ctx.ResponseHeaders())
}

func clear(adminClient keyvalue.TChanAdmin) {
	ctx, cancel := createContext()
	defer cancel()

	if err := adminClient.ClearAll(ctx); err != nil {
		switch err := err.(type) {
		case *keyvalue.NotAuthorized:
			log.Printf("You are not authorized to perform this method")
		default:
			log.Printf("ClearAll failed unexpectedly: %v", err)
		}
		return
	}

	log.Printf("ClearAll completed, all keys cleared")
}

func createContext() (thrift.Context, func()) {
	ctx, cancel := thrift.NewContext(time.Second)
	ctx = thrift.WithHeaders(ctx, map[string]string{"user": curUser})
	return ctx, cancel
}
