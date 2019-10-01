package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/stretchr/testify/assert"
)

var dataSource string
var insideCi = false
var insideDocker = false

const dbName = "dendrite_syncapi"

func init() {
	for _, val := range os.Environ() {
		tokens := strings.Split(val, "=")
		if tokens[0] == "CI" && tokens[1] == "true" {
			insideCi = true
		}
	}
	if !insideCi {
		if _, err := os.Open("/.dockerenv"); err == nil {
			insideDocker = true
		}
	}

	if insideCi {
		dataSource = fmt.Sprintf("postgres://postgres@localhost/%s?sslmode=disable", dbName)
	} else if insideDocker {
		dataSource = fmt.Sprintf("postgres://dendrite:itsasecret@postgres/%s?sslmode=disable", dbName)
	} else {
		dataSource = fmt.Sprintf("postgres://dendrite:itsasecret@localhost:15432/%s?sslmode=disable", dbName)
	}

	if insideCi {
		database := "dendrite_syncapi"
		cmd := exec.Command("psql", "postgres")
		cmd.Stdin = strings.NewReader(
			fmt.Sprintf("DROP DATABASE IF EXISTS %s; CREATE DATABASE %s;", database, database),
		)
		// Send stdout and stderr to our stderr so that we see error messages from
		// the psql process
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	}
}

const testEventID = "$test-event-id:test-domain.example.com"

func Test_sanityCheckOutputRoomEvents(t *testing.T) {
	dropTable(dataSource)
	db, err := NewSyncServerDatasource(dataSource)
	assert.Nil(t, err)

	err = db.events.prepare(db.db)
	assert.Nil(t, err)

	truncateTable(t, db)
	insertTestEvent(t, db)
	selectTestEvent(t, db)
	truncateTable(t, db)
}

func TestSyncServerDatasource_selectEventsWithEventIDs(t *testing.T) {
	db, err := NewSyncServerDatasource(dataSource)
	assert.Nil(t, err)
	truncateTable(t, db)
	insertTestEvent(t, db)
	ctx := context.Background()
	txn, err := db.db.Begin()
	assert.Nil(t, err)

	var eventIDs = []string{testEventID}
	events, err := db.fetchMissingStateEvents(ctx, txn, eventIDs)
	assert.Nil(t, err)
	assert.NotNil(t, events)
	assert.Condition(t, func() bool {
		return len(events) > 0
	})

	err = txn.Commit()
	assert.Nil(t, err)
}

func insertTestEvent(t *testing.T, db *SyncServerDatasource) {
	txn, err := db.db.Begin()
	assert.Nil(t, err)

	keyBytes := []byte("1122334455667788112233445566778811223344556677881122334455667788")
	eventBuilder := gomatrixserverlib.EventBuilder{
		RoomID:  "!test_room_id:test-domain.example.com",
		Content: []byte(`{"RawContent": "test-raw-content"}`),
		Sender:  "@test-user:test-domain.example.com",
	}
	event, err := eventBuilder.Build(
		testEventID,
		time.Now(),
		"test-domain.example.com",
		"test-key-id",
		keyBytes)

	assert.Nil(t, err)

	var addState, removeState []string
	transactionID := api.TransactionID{
		SessionID:     0x11223344,
		TransactionID: "test-transaction-id",
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

func selectTestEvent(t *testing.T, db *SyncServerDatasource) {
	ctx := context.Background()

	var eventIDs = []string{testEventID}
	res, err := db.Events(ctx, eventIDs)
	assert.Nil(t, err)
	assert.NotNil(t, res)

	assert.Condition(t, func() bool {
		return len(res) > 0
	})
}

func dropTable(dataSource string) {
	if db, err := sql.Open("postgres", dataSource); err == nil && db != nil {
		_, _ = db.Exec("DROP TABLE syncapi_output_room_events;")
	} else {
		panic("Error! Unable to refresh the database!")
	}
}

func truncateTable(t *testing.T, db *SyncServerDatasource) {
	_, err := db.db.Exec("TRUNCATE syncapi_output_room_events")
	assert.Nil(t, err)
}
