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

package http

import (
	"net/http"

	"github.com/uber/tchannel-go/typed"
)

func writeHeaders(wb *typed.WriteBuffer, form http.Header) {
	numHeadersDeferred := wb.DeferUint16()
	numHeaders := uint16(0)
	for k, values := range form {
		for _, v := range values {
			wb.WriteLen16String(k)
			wb.WriteLen16String(v)
			numHeaders++
		}
	}
	numHeadersDeferred.Update(numHeaders)
}

func readHeaders(rb *typed.ReadBuffer, form http.Header) {
	numHeaders := rb.ReadUint16()
	for i := 0; i < int(numHeaders); i++ {
		k := rb.ReadLen16String()
		v := rb.ReadLen16String()
		form[k] = append(form[k], v)
	}
}

func readVarintString(rb *typed.ReadBuffer) string {
	length := rb.ReadUvarint()
	return rb.ReadString(int(length))
}

func writeVarintString(wb *typed.WriteBuffer, s string) {
	wb.WriteUvarint(uint64(len(s)))
	wb.WriteString(s)
}
