package input_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/input"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
)

var js nats.JetStreamContext
var jc *nats.Conn

func TestMain(m *testing.M) {
	var pc *process.ProcessContext
	pc, js, jc = jetstream.PrepareForTests()
	code := m.Run()
	pc.ShutdownDendrite()
	pc.WaitForComponentsToFinish()
	os.Exit(code)
}

func TestSingleTransactionOnInput(t *testing.T) {
	deadline, _ := t.Deadline()
	if max := time.Now().Add(time.Second * 3); deadline.After(max) {
		deadline = max
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	event, err := gomatrixserverlib.NewEventFromTrustedJSON(
		[]byte(`{"auth_events":[],"content":{"creator":"@neilalexander:dendrite.matrix.org","room_version":"6"},"depth":1,"hashes":{"sha256":"jqOqdNEH5r0NiN3xJtj0u5XUVmRqq9YvGbki1wxxuuM"},"origin":"dendrite.matrix.org","origin_server_ts":1644595362726,"prev_events":[],"prev_state":[],"room_id":"!jSZZRknA6GkTBXNP:dendrite.matrix.org","sender":"@neilalexander:dendrite.matrix.org","signatures":{"dendrite.matrix.org":{"ed25519:6jB2aB":"bsQXO1wketf1OSe9xlndDIWe71W9KIundc6rBw4KEZdGPW7x4Tv4zDWWvbxDsG64sS2IPWfIm+J0OOozbrWIDw"}},"state_key":"","type":"m.room.create"}`),
		false, gomatrixserverlib.RoomVersionV6,
	)
	if err != nil {
		t.Fatal(err)
	}
	in := api.InputRoomEvent{
		Kind:  api.KindOutlier, // don't panic if we generate an output event
		Event: event.Headered(gomatrixserverlib.RoomVersionV6),
	}
	cache, err := caching.NewInMemoryLRUCache(false)
	if err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(
		&config.DatabaseOptions{
			ConnectionString:   "",
			MaxOpenConnections: 1,
			MaxIdleConnections: 1,
		},
		cache,
	)
	if err != nil {
		t.Logf("PostgreSQL not available (%s), skipping", err)
		t.SkipNow()
	}
	inputter := &input.Inputer{
		DB:         db,
		JetStream:  js,
		NATSClient: jc,
	}
	res := &api.InputRoomEventsResponse{}
	inputter.InputRoomEvents(
		ctx,
		&api.InputRoomEventsRequest{
			InputRoomEvents: []api.InputRoomEvent{in},
			Asynchronous:    false,
		},
		res,
	)
	// If we fail here then it's because we've hit the test deadline,
	// so we probably deadlocked
	if err := res.Err(); err != nil {
		t.Fatal(err)
	}
}
