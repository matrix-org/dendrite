package storage

import (
	"context"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

const dataSourceName = "postgres://dendrite:itsasecret@postgres/dendrite_syncapi?sslmode=disable"

//const dataSourceName = "postgres://dendrite:itsasecret@localhost:15432/dendrite_syncapi?sslmode=disable"

const testEventId = "test-event-id"

func Test_sanityCheckOutputRoomEvents(t *testing.T) {
	db, err := NewSyncServerDatabase(dataSourceName)
	assert.Nil(t, err)

	err = db.events.prepare(db.db)
	assert.Nil(t, err)

	truncateTable(t, db)
	insertTestEvent(t, db)
	selectTestEvent(t, db)
	truncateTable(t, db)
}

func TestSyncServerDatabase_selectEventsWithEventIDs(t *testing.T) {
	db, err := NewSyncServerDatabase(dataSourceName)
	assert.Nil(t, err)
	insertTestEvent(t, db)
	ctx := context.Background()
	txn, err := db.db.Begin()

	var eventIDs = []string{testEventId}
	events, err := db.fetchMissingStateEvents(ctx, txn, eventIDs)
	assert.Nil(t, err)
	assert.NotNil(t, events)
	assert.Condition(t, func() bool {
		return len(events) > 0
	})

}

func insertTestEvent(t *testing.T, db *SyncServerDatabase) {
	txn, err := db.db.Begin()
	assert.Nil(t, err)

	keyBytes := []byte("1122334455667788112233445566778811223344556677881122334455667788")
	eventBuilder := gomatrixserverlib.EventBuilder{
		RoomID:  "test-room-id",
		Content: []byte(`{"RawContent": "test-raw-content"}`),
	}
	event, err := eventBuilder.Build(
		testEventId,
		time.Now(),
		"test-server-name",
		"test-key-id",
		keyBytes)


	var addState, removeState []string
	transactionID := api.TransactionID{
		DeviceID: "test-device-id",
		TransactionID:"test-transaction-id",
	}

	newEventID, err := db.events.insertEvent(
		context.Background(),
		txn,
		&event,
		addState,
		removeState,
		&transactionID)

	assert.Nil(t, err)
	err = txn.Commit()
	assert.Nil(t, err)

	assert.Condition(t, func() bool {
		return newEventID > 0
	})
}

func selectTestEvent(t *testing.T, db *SyncServerDatabase) {
	ctx := context.Background()

	var eventIDs = []string{testEventId}
	res, err := db.Events(ctx, eventIDs)
	assert.Nil(t, err)
	assert.NotNil(t, res)

	assert.Condition(t, func() bool {
		return len(res) > 0
	})
}

func truncateTable(t *testing.T, db *SyncServerDatabase) {
	_, err := db.db.Exec("TRUNCATE syncapi_output_room_events")
	assert.Nil(t, err)
}
