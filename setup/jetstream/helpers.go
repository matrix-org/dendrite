package jetstream

import "github.com/nats-io/nats.go"

func WithJetStreamMessage(msg *nats.Msg, f func(msg *nats.Msg) bool) {
	if f(msg) {
		_ = msg.Ack()
	} else {
		_ = msg.Nak()
	}
}
