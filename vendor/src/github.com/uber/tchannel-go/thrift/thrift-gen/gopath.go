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
	"fmt"
	"os"
	"path/filepath"
)

// ResolveWithGoPath will resolve the filename relative to GOPATH and returns
// the first file that exists, or an error otherwise.
func ResolveWithGoPath(filename string) (string, error) {
	for _, file := range goPathCandidates(filename) {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			return file, nil
		}
	}

	return "", fmt.Errorf("file not found on GOPATH: %q", filename)
}

func goPathCandidates(filename string) []string {
	candidates := []string{filename}
	paths := filepath.SplitList(os.Getenv("GOPATH"))
	for _, path := range paths {
		resolvedFilename := filepath.Join(path, "src", filename)
		candidates = append(candidates, resolvedFilename)
	}

	return candidates
}
