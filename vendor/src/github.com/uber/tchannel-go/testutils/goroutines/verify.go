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

package goroutines

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

// filterStacks will filter any stacks excluded by the given VerifyOpts.
func filterStacks(stacks []Stack, skipID int, opts *VerifyOpts) []Stack {
	filtered := stacks[:0]
	for _, stack := range stacks {
		if stack.ID() == skipID || isTestStack(stack) {
			continue
		}
		if opts.ShouldSkip(stack) {
			continue
		}
		filtered = append(filtered, stack)
	}
	return filtered
}

func isTestStack(s Stack) bool {
	switch funcName := s.firstFunction; funcName {
	case "testing.RunTests", "testing.(*T).Run":
		return strings.HasPrefix(s.State(), "chan receive")
	case "runtime.goexit":
		return strings.HasPrefix(s.State(), "syscall")
	default:
		return false
	}
}

// IdentifyLeaks looks for extra goroutines, and returns a descriptive error if
// it finds any.
func IdentifyLeaks(opts *VerifyOpts) error {
	cur := GetCurrentStack().id

	const maxAttempts = 50
	var stacks []Stack
	for i := 0; i < maxAttempts; i++ {
		stacks = GetAll()
		stacks = filterStacks(stacks, cur, opts)

		if len(stacks) == 0 {
			return nil
		}

		if i > maxAttempts/2 {
			time.Sleep(time.Duration(i) * time.Millisecond)
		} else {
			runtime.Gosched()
		}
	}

	return fmt.Errorf("found unexpected goroutines:\n%s", stacks)
}

// VerifyNoLeaks calls IdentifyLeaks and fails the test if it finds any leaked
// goroutines.
func VerifyNoLeaks(t testing.TB, opts *VerifyOpts) {
	if err := IdentifyLeaks(opts); err != nil {
		t.Error(err.Error())
	}
}
