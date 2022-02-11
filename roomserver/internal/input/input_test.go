package input_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/input"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

func psqlConnectionString() config.DataSource {
	user := os.Getenv("POSTGRES_USER")
	if user == "" {
		user = "dendrite"
	}
	dbName := os.Getenv("POSTGRES_DB")
	if dbName == "" {
		dbName = "dendrite"
	}
	connStr := fmt.Sprintf(
		"user=%s dbname=%s sslmode=disable", user, dbName,
	)
	password := os.Getenv("POSTGRES_PASSWORD")
	if password != "" {
		connStr += fmt.Sprintf(" password=%s", password)
	}
	host := os.Getenv("POSTGRES_HOST")
	if host != "" {
		connStr += fmt.Sprintf(" host=%s", host)
	}
	return config.DataSource(connStr)
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
			ConnectionString:   psqlConnectionString(),
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
		DB: db,
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
