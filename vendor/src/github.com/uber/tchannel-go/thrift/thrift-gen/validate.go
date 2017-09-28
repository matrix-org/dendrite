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

	"github.com/samuel/go-thrift/parser"
)

// Validate validates that the given spec is supported by thrift-gen.
func Validate(svc *parser.Service) error {
	for _, m := range svc.Methods {
		if err := validateMethod(svc, m); err != nil {
			return err
		}
	}
	return nil
}

func validateMethod(svc *parser.Service, m *parser.Method) error {
	if m.Oneway {
		return fmt.Errorf("oneway methods are not supported: %s.%v", svc.Name, m.Name)
	}
	for _, arg := range m.Arguments {
		if arg.Optional {
			// Go treats argument structs as "Required" in the generated code interface.
			arg.Optional = false
		}
	}
	return nil
}
