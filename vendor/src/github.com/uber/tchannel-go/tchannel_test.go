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

package tchannel_test

import (
	"flag"
	"fmt"
	"os"
	"testing"

	. "github.com/uber/tchannel-go"

	"github.com/uber/tchannel-go/testutils/goroutines"
)

func checkAllChannels() error {
	ch, err := NewChannel("test-end", nil)
	if err != nil {
		return err
	}

	var foundChannels bool
	allChannels := ch.IntrospectOthers(&IntrospectionOptions{})
	for _, cs := range allChannels {
		if len(cs) != 0 {
			foundChannels = true
		}
	}

	if !foundChannels {
		return nil
	}

	return fmt.Errorf("unclosed channels:\n%+v", allChannels)
}

func TestMain(m *testing.M) {
	flag.Parse()
	exitCode := m.Run()

	if exitCode == 0 {
		// Only do extra checks if the tests were successful.
		if err := goroutines.IdentifyLeaks(nil); err != nil {
			fmt.Fprintf(os.Stderr, "Found goroutine leaks on successful test run: %v", err)
			exitCode = 1
		}

		if err := checkAllChannels(); err != nil {
			fmt.Fprintf(os.Stderr, "Found unclosed channels on successful test run: %v", err)
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}
