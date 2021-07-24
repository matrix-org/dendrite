package jetstream

import (
	"time"

	"github.com/nats-io/nats.go"
)

var (
	OutputRoomEvent         = "OutputRoomEvent"
	OutputSendToDeviceEvent = "OutputSendToDeviceEvent"
	OutputKeyChangeEvent    = "OutputKeyChangeEvent"
	OutputTypingEvent       = "OutputTypingEvent"
	OutputClientData        = "OutputClientData"
	OutputReceiptEvent      = "OutputReceiptEvent"
)

var streams = []*nats.StreamConfig{
	{
		Name:      OutputRoomEvent,
		Subjects:  []string{OutputRoomEvent},
		Retention: nats.InterestPolicy,
		Storage:   nats.FileStorage,
	},
	{
		Name:      OutputSendToDeviceEvent,
		Subjects:  []string{OutputSendToDeviceEvent},
		Retention: nats.InterestPolicy,
		Storage:   nats.FileStorage,
	},
	{
		Name:      OutputKeyChangeEvent,
		Subjects:  []string{OutputKeyChangeEvent},
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	},
	{
		Name:      OutputTypingEvent,
		Subjects:  []string{OutputTypingEvent},
		Retention: nats.InterestPolicy,
		Storage:   nats.MemoryStorage,
		MaxAge:    time.Second * 60,
	},
	{
		Name:      OutputClientData,
		Subjects:  []string{OutputClientData},
		Retention: nats.InterestPolicy,
		Storage:   nats.FileStorage,
	},
	{
		Name:      OutputReceiptEvent,
		Subjects:  []string{OutputReceiptEvent},
		Retention: nats.InterestPolicy,
		Storage:   nats.FileStorage,
	},
}
