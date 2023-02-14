package main

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
)

// This is an instrumented main, used when running integration tests (sytest) with code coverage.
// Compile: go test -c -race -cover -covermode=atomic -o monolith.debug -coverpkg "github.com/matrix-org/..." ./cmd/dendrite-monolith-server
// Run the monolith: ./monolith.debug -test.coverprofile=/somewhere/to/dump/integrationcover.out DEVEL --config dendrite.yaml
// Generate HTML with coverage: go tool cover -html=/somewhere/where/there/is/integrationcover.out -o cover.html
// Source: https://dzone.com/articles/measuring-integration-test-coverage-rate-in-pouchc
func TestMain(_ *testing.T) {
	var (
		args []string
	)

	for _, arg := range os.Args {
		switch {
		case strings.HasPrefix(arg, "DEVEL"):
		case strings.HasPrefix(arg, "-test"):
		default:
			args = append(args, arg)
		}
	}
	// only run the tests if there are args to be passed
	if len(args) <= 1 {
		return
	}

	waitCh := make(chan int, 1)
	os.Args = args
	go func() {
		main()
		close(waitCh)
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP)

	select {
	case <-signalCh:
		return
	case <-waitCh:
		return
	}
}
