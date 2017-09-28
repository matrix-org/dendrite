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

package tchannel

import (
	"testing"
	"time"

	"github.com/uber/tchannel-go/typed"

	"github.com/stretchr/testify/assert"
)

type testCallReq int

const (
	reqHasHeaders testCallReq = (1 << iota)
	reqHasCaller
	reqHasDelegate
	reqHasRoutingKey
	reqHasChecksum
	reqTotalCombinations
	reqHasAll testCallReq = reqTotalCombinations - 1
)

func (cr testCallReq) req() lazyCallReq {
	// TODO: Constructing a frame is ugly because the initial flags byte is
	// written in reqResWriter instead of callReq. We should instead handle that
	// in callReq, which will allow our tests to be sane.
	f := NewFrame(200)
	fh := fakeHeader()
	f.Header = fh
	fh.write(typed.NewWriteBuffer(f.headerBuffer))

	payload := typed.NewWriteBuffer(f.Payload)
	payload.WriteSingleByte(0)           // flags
	payload.WriteUint32(42)              // TTL
	payload.WriteBytes(make([]byte, 25)) // tracing
	payload.WriteLen8String("bankmoji")  // service

	headers := make(map[string]string)
	if cr&reqHasHeaders != 0 {
		addRandomHeaders(headers)
	}
	if cr&reqHasCaller != 0 {
		headers["cn"] = "fake-caller"
	}
	if cr&reqHasDelegate != 0 {
		headers["rd"] = "fake-delegate"
	}
	if cr&reqHasRoutingKey != 0 {
		headers["rk"] = "fake-routingkey"
	}
	writeHeaders(payload, headers)

	if cr&reqHasChecksum == 0 {
		payload.WriteSingleByte(byte(ChecksumTypeNone)) // checksum type
		// no checksum contents for None
	} else {
		payload.WriteSingleByte(byte(ChecksumTypeCrc32C)) // checksum type
		payload.WriteUint32(0)                            // checksum contents
	}
	payload.WriteLen16String("moneys") // method
	return newLazyCallReq(f)
}

func withLazyCallReqCombinations(f func(cr testCallReq)) {
	for cr := testCallReq(0); cr < reqTotalCombinations; cr++ {
		f(cr)
	}
}

type testCallRes int

const (
	resIsContinued testCallRes = (1 << iota)
	resIsOK
	resHasHeaders
	resHasChecksum
	resTotalCombinations
)

func (cr testCallRes) res() lazyCallRes {
	f := NewFrame(100)
	fh := FrameHeader{
		size:        uint16(0xFF34),
		messageType: messageTypeCallRes,
		ID:          0xDEADBEEF,
	}
	f.Header = fh
	fh.write(typed.NewWriteBuffer(f.headerBuffer))

	payload := typed.NewWriteBuffer(f.Payload)

	if cr&resIsContinued == 0 {
		payload.WriteSingleByte(0) // flags
	} else {
		payload.WriteSingleByte(hasMoreFragmentsFlag) // flags
	}

	if cr&resIsOK == 0 {
		payload.WriteSingleByte(1) // code not ok
	} else {
		payload.WriteSingleByte(0) // code ok
	}

	headers := make(map[string]string)
	if cr&resHasHeaders != 0 {
		addRandomHeaders(headers)
	}
	writeHeaders(payload, headers)

	if cr&resHasChecksum == 0 {
		payload.WriteSingleByte(byte(ChecksumTypeNone)) // checksum type
		// No contents for ChecksumTypeNone.
	} else {
		payload.WriteSingleByte(byte(ChecksumTypeCrc32C)) // checksum type
		payload.WriteUint32(0)                            // checksum contents
	}
	payload.WriteUint16(0) // no arg1 for call res
	return newLazyCallRes(f)
}

func withLazyCallResCombinations(f func(cr testCallRes)) {
	for cr := testCallRes(0); cr < resTotalCombinations; cr++ {
		f(cr)
	}
}

func (ec SystemErrCode) fakeErrFrame() lazyError {
	f := NewFrame(100)
	fh := FrameHeader{
		size:        uint16(0xFF34),
		messageType: messageTypeError,
		ID:          invalidMessageID,
	}
	f.Header = fh
	fh.write(typed.NewWriteBuffer(f.headerBuffer))

	payload := typed.NewWriteBuffer(f.Payload)
	payload.WriteSingleByte(byte(ec))
	payload.WriteBytes(make([]byte, 25)) // tracing

	msg := ec.String()
	payload.WriteUint16(uint16(len(msg)))
	payload.WriteBytes([]byte(msg))
	return newLazyError(f)
}

func withLazyErrorCombinations(f func(ec SystemErrCode)) {
	codes := []SystemErrCode{
		ErrCodeInvalid,
		ErrCodeTimeout,
		ErrCodeCancelled,
		ErrCodeBusy,
		ErrCodeDeclined,
		ErrCodeUnexpected,
		ErrCodeBadRequest,
		ErrCodeNetwork,
		ErrCodeProtocol,
	}
	for _, ec := range codes {
		f(ec)
	}
}

func addRandomHeaders(headers map[string]string) {
	headers["k1"] = "v1"
	headers["k222222"] = ""
	headers["k3"] = "thisisalonglongkey"
}

func writeHeaders(w *typed.WriteBuffer, headers map[string]string) {
	w.WriteSingleByte(byte(len(headers))) // number of headers
	for k, v := range headers {
		w.WriteLen8String(k)
		w.WriteLen8String(v)
	}
}

func assertWrappingPanics(t testing.TB, f *Frame, wrap func(f *Frame)) {
	assert.Panics(t, func() {
		wrap(f)
	}, "Should panic when wrapping an unexpected frame type.")
}

func TestLazyCallReqRejectsOtherFrames(t *testing.T) {
	assertWrappingPanics(
		t,
		resIsContinued.res().Frame,
		func(f *Frame) { newLazyCallReq(f) },
	)
}

func TestLazyCallReqService(t *testing.T) {
	withLazyCallReqCombinations(func(crt testCallReq) {
		cr := crt.req()
		assert.Equal(t, "bankmoji", string(cr.Service()), "Service name mismatch")
	})
}

func TestLazyCallReqCaller(t *testing.T) {
	withLazyCallReqCombinations(func(crt testCallReq) {
		cr := crt.req()
		if crt&reqHasCaller == 0 {
			assert.Equal(t, []byte(nil), cr.Caller(), "Unexpected caller name.")
		} else {
			assert.Equal(t, "fake-caller", string(cr.Caller()), "Caller name mismatch")
		}
	})
}

func TestLazyCallReqRoutingDelegate(t *testing.T) {
	withLazyCallReqCombinations(func(crt testCallReq) {
		cr := crt.req()
		if crt&reqHasDelegate == 0 {
			assert.Equal(t, []byte(nil), cr.RoutingDelegate(), "Unexpected routing delegate.")
		} else {
			assert.Equal(t, "fake-delegate", string(cr.RoutingDelegate()), "Routing delegate mismatch.")
		}
	})
}

func TestLazyCallReqRoutingKey(t *testing.T) {
	withLazyCallReqCombinations(func(crt testCallReq) {
		cr := crt.req()
		if crt&reqHasRoutingKey == 0 {
			assert.Equal(t, []byte(nil), cr.RoutingKey(), "Unexpected routing key.")
		} else {
			assert.Equal(t, "fake-routingkey", string(cr.RoutingKey()), "Routing key mismatch.")
		}
	})
}

func TestLazyCallReqMethod(t *testing.T) {
	withLazyCallReqCombinations(func(crt testCallReq) {
		cr := crt.req()
		assert.Equal(t, "moneys", string(cr.Method()), "Method name mismatch")
	})
}

func TestLazyCallReqTTL(t *testing.T) {
	withLazyCallReqCombinations(func(crt testCallReq) {
		cr := crt.req()
		assert.Equal(t, 42*time.Millisecond, cr.TTL(), "Failed to parse TTL from frame.")
	})
}

func TestLazyCallReqSetTTL(t *testing.T) {
	withLazyCallReqCombinations(func(crt testCallReq) {
		cr := crt.req()
		cr.SetTTL(time.Second)
		assert.Equal(t, time.Second, cr.TTL(), "Failed to write TTL to frame.")
	})
}

func TestLazyCallResRejectsOtherFrames(t *testing.T) {
	assertWrappingPanics(
		t,
		reqHasHeaders.req().Frame,
		func(f *Frame) { newLazyCallRes(f) },
	)
}

func TestLazyCallResOK(t *testing.T) {
	withLazyCallResCombinations(func(crt testCallRes) {
		cr := crt.res()
		if crt&resIsOK == 0 {
			assert.False(t, cr.OK(), "Expected call res to have a non-ok code.")
		} else {
			assert.True(t, cr.OK(), "Expected call res to have code ok.")
		}
	})
}

func TestLazyErrorRejectsOtherFrames(t *testing.T) {
	assertWrappingPanics(
		t,
		reqHasHeaders.req().Frame,
		func(f *Frame) { newLazyError(f) },
	)
}

func TestLazyErrorCodes(t *testing.T) {
	withLazyErrorCombinations(func(ec SystemErrCode) {
		f := ec.fakeErrFrame()
		assert.Equal(t, ec, f.Code(), "Mismatch between error code and lazy frame's Code() method.")
	})
}
