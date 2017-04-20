package main

import (
	"flag"
	"fmt"
	"os"
)

const usage = `Usage: %s

Create a single endpoint URL which clients can be pointed at.

The client-server API in Dendrite is split across multiple processes
which listen on multiple ports. You cannot point a Matrix client at
any of those ports, as there will be unimplemented functionality.
In addition, all client-server API processes start with the additional
path prefix '/api', which Matrix clients will be unaware of.

This tool will proxy requests for all client-server URLs and forward
them to their respective process. It will also add the '/api' path
prefix to incoming requests.

Arguments:

`

var (
	syncServerURL = flag.String("sync-server-url", "", "The base URL of the listening 'dendrite-sync-server' process. E.g. 'http://localhost:4200'")
	clientAPIURL  = flag.String("client-api-url", "", "The base URL of the listening 'dendrite-client-api' process. E.g. 'http://localhost:4321'")
	bindAddress   = flag.String("bind-address", ":8008", "The listening port for the proxy.")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if *syncServerURL == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "no --sync-server-url specified.")
		os.Exit(1)
	}

	if *clientAPIURL == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "no --client-api-url specified.")
		os.Exit(1)
	}

	fmt.Println("Proxying requests to:")
	fmt.Println("  /_matrix/client/r0/sync  => ", *syncServerURL+"/api/_matrix/client/r0/sync")
	fmt.Println("  /*                       => ", *clientAPIURL+"/api/*")
	fmt.Println("Listening on %s", *bindAddress)

}
