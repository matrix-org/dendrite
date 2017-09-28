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

import "github.com/samuel/go-thrift/parser"

// Include represents a single include statement in the Thrift file.
type Include struct {
	key  string
	file string
	pkg  string
}

// Import returns the go import to use for this package.
func (i *Include) Import() string {
	// TODO(prashant): Rename imports so they don't clash with standard imports.
	// This is not high priority since Apache thrift clashes already with "bytes" and "fmt".
	// which are the same imports we would clash with.
	return *packagePrefix + i.Package()
}

// Package returns the package selector for this package.
func (i *Include) Package() string {
	return i.pkg
}

func createIncludes(parsed *parser.Thrift, all map[string]parseState) map[string]*Include {
	includes := make(map[string]*Include)
	for k, v := range parsed.Includes {
		included := all[v]
		includes[k] = &Include{
			key:  k,
			file: v,
			pkg:  included.namespace,
		}
	}
	return includes
}
