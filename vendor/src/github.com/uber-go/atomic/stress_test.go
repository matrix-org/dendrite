// Copyright (c) 2016 Uber Technologies, Inc.
//
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

package atomic

import (
	"runtime"
	"sync"
	"testing"
)

const (
	_parallelism = 4
	_iterations  = 1000
)

var _stressTests = map[string]func(){
	"i32":    stressInt32,
	"i64":    stressInt64,
	"u32":    stressUint32,
	"u64":    stressUint64,
	"f64":    stressFloat64,
	"bool":   stressBool,
	"string": stressString,
}

func TestStress(t *testing.T) {
	for name, f := range _stressTests {
		t.Run(name, func(t *testing.T) {
			defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(_parallelism))

			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(_parallelism)
			for i := 0; i < _parallelism; i++ {
				go func() {
					defer wg.Done()
					<-start
					for j := 0; j < _iterations; j++ {
						f()
					}
				}()
			}
			close(start)
			wg.Wait()
		})
	}
}

func BenchmarkStress(b *testing.B) {
	for name, f := range _stressTests {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				f()
			}
		})
	}
}

func stressInt32() {
	var atom Int32
	atom.Load()
	atom.Add(1)
	atom.Sub(2)
	atom.Inc()
	atom.Dec()
	atom.CAS(1, 0)
	atom.Swap(5)
	atom.Store(1)
}

func stressInt64() {
	var atom Int64
	atom.Load()
	atom.Add(1)
	atom.Sub(2)
	atom.Inc()
	atom.Dec()
	atom.CAS(1, 0)
	atom.Swap(5)
	atom.Store(1)
}

func stressUint32() {
	var atom Uint32
	atom.Load()
	atom.Add(1)
	atom.Sub(2)
	atom.Inc()
	atom.Dec()
	atom.CAS(1, 0)
	atom.Swap(5)
	atom.Store(1)
}

func stressUint64() {
	var atom Uint64
	atom.Load()
	atom.Add(1)
	atom.Sub(2)
	atom.Inc()
	atom.Dec()
	atom.CAS(1, 0)
	atom.Swap(5)
	atom.Store(1)
}

func stressFloat64() {
	var atom Float64
	atom.Load()
	atom.CAS(1.0, 0.1)
	atom.Add(1.1)
	atom.Sub(0.2)
	atom.Store(1.0)
}

func stressBool() {
	var atom Bool
	atom.Load()
	atom.Store(false)
	atom.Swap(true)
	atom.CAS(true, false)
	atom.CAS(true, false)
	atom.Load()
	atom.Toggle()
	atom.Toggle()
}

func stressString() {
	var atom String
	atom.Load()
	atom.Store("abc")
	atom.Load()
	atom.Store("def")
	atom.Load()
	atom.Store("")
}
