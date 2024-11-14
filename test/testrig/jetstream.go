package testrig

import (
	"encoding/json"
	"testing"

	"github.com/element-hq/dendrite/setup/config"
	"github.com/nats-io/nats.go"

	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/setup/jetstream"
)

func MustPublishMsgs(t *testing.T, jsctx nats.JetStreamContext, msgs ...*nats.Msg) {
	t.Helper()
	for _, msg := range msgs {
		if _, err := jsctx.PublishMsg(msg); err != nil {
			t.Fatalf("MustPublishMsgs: failed to publish message: %s", err)
		}
	}
}

func NewOutputEventMsg(t *testing.T, cfg *config.Dendrite, roomID string, update api.OutputEvent) *nats.Msg {
	t.Helper()
	msg := nats.NewMsg(cfg.Global.JetStream.Prefixed(jetstream.OutputRoomEvent))
	msg.Header.Set(jetstream.RoomEventType, string(update.Type))
	msg.Header.Set(jetstream.RoomID, roomID)
	var err error
	msg.Data, err = json.Marshal(update)
	if err != nil {
		t.Fatalf("failed to marshal update: %s", err)
	}
	return msg
}
