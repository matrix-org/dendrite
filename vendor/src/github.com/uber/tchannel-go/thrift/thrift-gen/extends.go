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
	"sort"
	"strings"
)

// setExtends will set the ExtendsService for all services.
// It is done after all files are parsed, as services may extend those
// found in an included file.
func setExtends(state map[string]parseState) error {
	for _, v := range state {
		for _, s := range v.services {
			if s.Extends == "" {
				continue
			}

			var searchServices []*Service
			var searchFor string
			parts := strings.SplitN(s.Extends, ".", 2)
			// If it's not imported, then look at the current file's services.
			if len(parts) < 2 {
				searchServices = v.services
				searchFor = s.Extends
			} else {
				include := v.global.includes[parts[0]]
				s.ExtendsPrefix = include.pkg + "."
				searchServices = state[include.file].services
				searchFor = parts[1]
			}

			foundService := sort.Search(len(searchServices), func(i int) bool {
				return searchServices[i].Name >= searchFor
			})
			if foundService == len(searchServices) {
				return fmt.Errorf("failed to find base service %q for %q", s.Extends, s.Name)
			}
			s.ExtendsService = searchServices[foundService]
		}
	}

	return nil
}
