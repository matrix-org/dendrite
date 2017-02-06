package storage

import (
	"database/sql"
	// Import the postgres database driver.
	_ "github.com/lib/pq"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// A Database is used to store room events and stream offsets.
type Database struct {
	statements statements
	db         *sql.DB
}

// Open a postgres database.
func Open(dataSourceName string) (*Database, error) {
	var d Database
	var err error
	if d.db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
	if err = d.statements.prepare(d.db); err != nil {
		return nil, err
	}
	return &d, nil
}

// PartitionOffsets implements input.ConsumerDatabase
func (d *Database) PartitionOffsets(topic string) ([]types.PartitionOffset, error) {
	return d.statements.selectPartitionOffsets(topic)
}

// SetPartitionOffset implements input.ConsumerDatabase
func (d *Database) SetPartitionOffset(topic string, partition int32, offset int64) error {
	return d.statements.upsertPartitionOffset(topic, partition, offset)
}

// StoreEvent implements input.EventDatabase
func (d *Database) StoreEvent(event gomatrixserverlib.Event) error {
	var (
		roomNID          int64
		eventTypeNID     int64
		eventStateKeyNID int64
		eventNID         int64
		err              error
	)

	if roomNID, err = d.assignRoomNID(event.RoomID()); err != nil {
		return err
	}

	if eventTypeNID, err = d.assignEventTypeNID(event.Type()); err != nil {
		return err
	}

	eventStateKey := event.StateKey()
	if eventStateKey != nil {
		if eventStateKeyNID, err = d.assignStateKeyNID(*eventStateKey); err != nil {
			return err
		}
	}

	eventID := event.EventID()
	referenceSHA256 := event.EventReference().EventSHA256

	if eventNID, err = d.statements.insertEvent(roomNID, eventTypeNID, eventStateKeyNID, eventID, referenceSHA256); err != nil {
		return err
	}

	return d.statements.insertEventJSON(eventNID, event.JSON())
}

func (d *Database) assignRoomNID(roomID string) (roomNID int64, err error) {
	if roomNID, err = d.statements.selectRoomNID(roomID); err != nil {
		return
	}
	if roomNID == 0 {
		return d.statements.insertRoomNID(roomID)
	}
	return
}

func (d *Database) assignEventTypeNID(eventType string) (eventTypeNID int64, err error) {
	if eventTypeNID, err = d.statements.selectEventTypeNID(eventType); err != nil {
		return
	}
	if eventTypeNID == 0 {
		return d.statements.insertEventTypeNID(eventType)
	}
	return
}

func (d *Database) assignStateKeyNID(eventStateKey string) (eventStateKeyNID int64, err error) {
	if eventStateKeyNID, err = d.statements.selectEventStateKeyNID(eventStateKey); err != nil {
		return
	}
	if eventStateKeyNID == 0 {
		return d.statements.insertEventTypeNID(eventStateKey)
	}
	return
}
