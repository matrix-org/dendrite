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
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/uber/tchannel-go/typed"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitReq(t *testing.T) {
	req := initReq{
		initMessage{
			id:      0xDEADBEEF,
			Version: 0x02,
			initParams: initParams{
				"lang": "en_US",
				"tz":   "GMT",
			},
		},
	}

	assert.Equal(t, uint32(0xDEADBEEF), req.ID(), "ids do not match")
	assert.Equal(t, messageTypeInitReq, req.messageType(), "types do not match")
	assertRoundTrip(t, &req, &initReq{initMessage{id: 0xDEADBEEF}})
}

func TestInitRes(t *testing.T) {
	res := initRes{
		initMessage{
			id:      0xDEADBEEF,
			Version: 0x04,
			initParams: initParams{
				"lang": "en_US",
				"tz":   "GMT",
			},
		},
	}

	assert.Equal(t, uint32(0xDEADBEEF), res.ID(), "ids do not match")
	assert.Equal(t, messageTypeInitRes, res.messageType(), "types do not match")
	assertRoundTrip(t, &res, &initRes{initMessage{id: 0xDEADBEEF}})
}

func TestCallReq(t *testing.T) {
	r := callReq{
		id:         0xDEADBEEF,
		TimeToLive: time.Second * 45,
		Tracing: Span{
			traceID:  294390430934,
			parentID: 398348934,
			spanID:   12762782,
			flags:    0x01,
		},
		Headers: transportHeaders{
			"r": "c",
			"f": "d",
		},
		Service: "udr",
	}

	assert.Equal(t, uint32(0xDEADBEEF), r.ID())
	assert.Equal(t, messageTypeCallReq, r.messageType())
	assertRoundTrip(t, &r, &callReq{id: 0xDEADBEEF})
}

func TestCallReqContinue(t *testing.T) {
	r := callReqContinue{
		id: 0xDEADBEEF,
	}

	assert.Equal(t, uint32(0xDEADBEEF), r.ID())
	assert.Equal(t, messageTypeCallReqContinue, r.messageType())
	assertRoundTrip(t, &r, &callReqContinue{id: 0xDEADBEEF})
}

func TestCallRes(t *testing.T) {
	r := callRes{
		id:           0xDEADBEEF,
		ResponseCode: responseApplicationError,
		Headers: transportHeaders{
			"r": "c",
			"f": "d",
		},
		Tracing: Span{
			traceID:  294390430934,
			parentID: 398348934,
			spanID:   12762782,
			flags:    0x04,
		},
	}

	assert.Equal(t, uint32(0xDEADBEEF), r.ID())
	assert.Equal(t, messageTypeCallRes, r.messageType())
	assertRoundTrip(t, &r, &callRes{id: 0xDEADBEEF})
}

func TestCallResContinue(t *testing.T) {
	r := callResContinue{
		id: 0xDEADBEEF,
	}

	assert.Equal(t, uint32(0xDEADBEEF), r.ID())
	assert.Equal(t, messageTypeCallResContinue, r.messageType())
	assertRoundTrip(t, &r, &callResContinue{id: 0xDEADBEEF})
}

func TestErrorMessage(t *testing.T) {
	m := errorMessage{
		errCode: ErrCodeBusy,
		message: "go away",
	}

	assert.Equal(t, messageTypeError, m.messageType())
	assertRoundTrip(t, &m, &errorMessage{})
}

func assertRoundTrip(t *testing.T, expected message, actual message) {
	w := typed.NewWriteBufferWithSize(1024)
	require.Nil(t, expected.write(w), fmt.Sprintf("error writing message %v", expected.messageType()))

	var b bytes.Buffer
	w.FlushTo(&b)

	r := typed.NewReadBufferWithSize(1024)
	_, err := r.FillFrom(bytes.NewReader(b.Bytes()), len(b.Bytes()))
	require.Nil(t, err)
	require.Nil(t, actual.read(r), fmt.Sprintf("error reading message %v", expected.messageType()))

	assert.Equal(t, expected, actual, fmt.Sprintf("pre- and post-marshal %v do not match", expected.messageType()))
}
