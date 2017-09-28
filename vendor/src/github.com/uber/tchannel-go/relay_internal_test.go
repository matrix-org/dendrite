package tchannel

import (
	"testing"

	"github.com/uber/tchannel-go/typed"

	"github.com/stretchr/testify/assert"
)

func TestFinishesCallResponses(t *testing.T) {
	tests := []struct {
		msgType      messageType
		flags        byte
		finishesCall bool
	}{
		{messageTypeCallRes, 0x00, true},
		{messageTypeCallRes, 0x01, false},
		{messageTypeCallRes, 0x02, true},
		{messageTypeCallRes, 0x03, false},
		{messageTypeCallRes, 0x04, true},
		{messageTypeCallResContinue, 0x00, true},
		{messageTypeCallResContinue, 0x01, false},
		{messageTypeCallResContinue, 0x02, true},
		{messageTypeCallResContinue, 0x03, false},
		{messageTypeCallResContinue, 0x04, true},
		// By definition, callreq should never terminate an RPC.
		{messageTypeCallReq, 0x00, false},
		{messageTypeCallReq, 0x01, false},
		{messageTypeCallReq, 0x02, false},
		{messageTypeCallReq, 0x03, false},
		{messageTypeCallReq, 0x04, false},
	}
	for _, tt := range tests {
		f := NewFrame(100)
		fh := FrameHeader{
			size:        uint16(0xFF34),
			messageType: tt.msgType,
			ID:          0xDEADBEEF,
		}
		f.Header = fh
		fh.write(typed.NewWriteBuffer(f.headerBuffer))

		payload := typed.NewWriteBuffer(f.Payload)
		payload.WriteSingleByte(tt.flags)
		assert.Equal(t, tt.finishesCall, finishesCall(f), "Wrong isLast for flags %v and message type %v", tt.flags, tt.msgType)
	}
}
