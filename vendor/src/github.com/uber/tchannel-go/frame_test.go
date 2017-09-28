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
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"testing"
	"testing/iotest"
	"testing/quick"

	"github.com/uber/tchannel-go/testutils/testreader"
	"github.com/uber/tchannel-go/typed"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeHeader() FrameHeader {
	return FrameHeader{
		size:        uint16(0xFF34),
		messageType: messageTypeCallReq,
		ID:          0xDEADBEEF,
	}
}

func TestFrameHeaderJSON(t *testing.T) {
	fh := fakeHeader()
	logged, err := json.Marshal(fh)
	assert.NoError(t, err, "FrameHeader can't be marshalled to JSON")
	assert.Equal(
		t,
		string(logged),
		`{"id":3735928559,"msgType":3,"size":65332}`,
		"FrameHeader didn't marshal to JSON as expected",
	)
}

func TestFraming(t *testing.T) {
	fh := fakeHeader()
	wbuf := typed.NewWriteBufferWithSize(1024)
	require.Nil(t, fh.write(wbuf))

	var b bytes.Buffer
	if _, err := wbuf.FlushTo(&b); err != nil {
		require.Nil(t, err)
	}

	rbuf := typed.NewReadBuffer(b.Bytes())

	var fh2 FrameHeader
	require.Nil(t, fh2.read(rbuf))

	assert.Equal(t, fh, fh2)
}

func TestPartialRead(t *testing.T) {
	f := NewFrame(MaxFramePayloadSize)
	f.Header.size = FrameHeaderSize + 2134
	f.Header.messageType = messageTypeCallReq
	f.Header.ID = 0xDEADBEED

	// We set the full payload but only the first 2134 bytes should be written.
	for i := 0; i < len(f.Payload); i++ {
		val := (i * 37) % 256
		f.Payload[i] = byte(val)
	}
	buf := &bytes.Buffer{}
	require.NoError(t, f.WriteOut(buf))
	assert.Equal(t, f.Header.size, uint16(buf.Len()), "frame size should match written bytes")

	// Read the data back, from a reader that fragments.
	f2 := NewFrame(MaxFramePayloadSize)
	require.NoError(t, f2.ReadIn(iotest.OneByteReader(buf)))

	// Ensure header and payload are the same.
	require.Equal(t, f.Header, f2.Header, "frame headers don't match")
	require.Equal(t, f.SizedPayload(), f2.SizedPayload(), "payload does not match")
}

func TestEmptyPayload(t *testing.T) {
	f := NewFrame(MaxFramePayloadSize)
	m := &pingRes{id: 1}
	require.NoError(t, f.write(m))

	// Write out the frame.
	buf := &bytes.Buffer{}
	require.NoError(t, f.WriteOut(buf))
	assert.Equal(t, FrameHeaderSize, buf.Len())

	// Read the frame from the buffer.
	// net.Conn returns io.EOF if you try to read 0 bytes at the end.
	// This is also simulated by the LimitedReader so we use that here.
	require.NoError(t, f.ReadIn(&io.LimitedReader{R: buf, N: FrameHeaderSize}))
}

func TestReservedBytes(t *testing.T) {
	// Set up a frame with non-zero values
	f := NewFrame(MaxFramePayloadSize)
	reader := testreader.Looper([]byte{^byte(0)})
	io.ReadFull(reader, f.Payload)
	f.Header.read(typed.NewReadBuffer(f.Payload))

	m := &pingRes{id: 1}
	f.write(m)

	buf := &bytes.Buffer{}
	f.WriteOut(buf)
	assert.Equal(t,
		[]byte{
			0x0, 0x10, // size
			0xd1,               // type
			0x0,                // reserved should always be 0
			0x0, 0x0, 0x0, 0x1, // id
			0x0, 0x0, 0x0, 0x0, // reserved should always be 0
			0x0, 0x0, 0x0, 0x0, // reserved should always be 0
		},
		buf.Bytes(), "Unexpected bytes")
}

func TestMessageType(t *testing.T) {
	frame := NewFrame(MaxFramePayloadSize)
	err := frame.write(&callReq{Service: "foo"})
	require.NoError(t, err, "Error writing message to frame.")
	assert.Equal(t, messageTypeCallReq, frame.messageType(), "Failed to read message type from frame.")
}

func TestFrameReadIn(t *testing.T) {
	maxPayload := bytes.Repeat([]byte{1}, MaxFramePayloadSize)
	tests := []struct {
		msg              string
		bs               []byte
		wantFrameHeader  FrameHeader
		wantFramePayload []byte
		wantErr          string
	}{
		{
			msg: "frame with no payload",
			bs: []byte{
				0, 16 /* size */, 1 /* type */, 2 /* reserved */, 0, 0, 0, 3, /* id */
				9, 8, 7, 6, 5, 4, 3, 2, // reserved
			},
			wantFrameHeader: FrameHeader{
				size:        16,
				messageType: 1,
				reserved1:   2,
				ID:          3,
				// reserved:    [8]byte{9, 8, 7, 6, 5, 4, 3, 2}, // currently ignored.
			},
			wantFramePayload: []byte{},
		},
		{
			msg: "frame with small payload",
			bs: []byte{
				0, 18 /* size */, 1 /* type */, 2 /* reserved */, 0, 0, 0, 3, /* id */
				9, 8, 7, 6, 5, 4, 3, 2, // reserved
				100, 200, // payload
			},
			wantFrameHeader: FrameHeader{
				size:        18,
				messageType: 1,
				reserved1:   2,
				ID:          3,
				// reserved:    [8]byte{9, 8, 7, 6, 5, 4, 3, 2}, // currently ignored.
			},
			wantFramePayload: []byte{100, 200},
		},
		{
			msg: "frame with max size",
			bs: append([]byte{
				math.MaxUint8, math.MaxUint8 /* size */, 1 /* type */, 2 /* reserved */, 0, 0, 0, 3, /* id */
				9, 8, 7, 6, 5, 4, 3, 2, // reserved
			}, maxPayload...),
			wantFrameHeader: FrameHeader{
				size:        math.MaxUint16,
				messageType: 1,
				reserved1:   2,
				ID:          3,
				// currently ignored.
				// reserved:    [8]byte{9, 8, 7, 6, 5, 4, 3, 2},
			},
			wantFramePayload: maxPayload,
		},
		{
			msg: "frame with 0 size",
			bs: []byte{
				0, 0 /* size */, 1 /* type */, 2 /* reserved */, 0, 0, 0, 3, /* id */
				9, 8, 7, 6, 5, 4, 3, 2, // reserved
			},
			wantErr: "invalid frame size 0",
		},
		{
			msg: "frame with size < HeaderSize",
			bs: []byte{
				0, 15 /* size */, 1 /* type */, 2 /* reserved */, 0, 0, 0, 3, /* id */
				9, 8, 7, 6, 5, 4, 3, 2, // reserved
			},
			wantErr: "invalid frame size 15",
		},
		{
			msg: "frame with partial header",
			bs: []byte{
				0, 16 /* size */, 1 /* type */, 2 /* reserved */, 0, 0, 0, 3, /* id */
				// missing reserved bytes
			},
			wantErr: "unexpected EOF",
		},
		{
			msg: "frame with partial payload",
			bs: []byte{
				0, 24 /* size */, 1 /* type */, 2 /* reserved */, 0, 0, 0, 3, /* id */
				9, 8, 7, 6, 5, 4, 3, 2, // reserved
				1, 2, // partial payload
			},
			wantErr: "unexpected EOF",
		},
	}

	for _, tt := range tests {
		f := DefaultFramePool.Get()
		r := bytes.NewReader(tt.bs)
		err := f.ReadIn(r)
		if tt.wantErr != "" {
			require.Error(t, err, tt.msg)
			assert.Contains(t, err.Error(), tt.wantErr, tt.msg)
			continue
		}

		require.NoError(t, err, tt.msg)
		assert.Equal(t, tt.wantFrameHeader, f.Header, "%v: header mismatch", tt.msg)
		assert.Equal(t, tt.wantFramePayload, f.SizedPayload(), "%v: unexpected payload")
	}
}

func frameReadIn(bs []byte) (decoded bool) {
	frame := DefaultFramePool.Get()
	defer DefaultFramePool.Release(frame)

	defer func() {
		if r := recover(); r != nil {
			decoded = false
		}
	}()
	frame.ReadIn(bytes.NewReader(bs))
	return true
}

func TestQuickFrameReadIn(t *testing.T) {
	// Try to read any set of bytes as a frame.
	err := quick.Check(frameReadIn, &quick.Config{MaxCount: 10000})
	require.NoError(t, err, "Failed to fuzz test ReadIn")

	// Limit the search space to just headers.
	err = quick.Check(func(size uint16, t byte, id uint32) bool {
		bs := make([]byte, FrameHeaderSize)
		binary.BigEndian.PutUint16(bs[0:2], size)
		bs[2] = t
		binary.BigEndian.PutUint32(bs[4:8], id)
		return frameReadIn(bs)
	}, &quick.Config{MaxCount: 10000})
	require.NoError(t, err, "Failed to fuzz test ReadIn")
}
