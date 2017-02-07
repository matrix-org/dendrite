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
	// Assigned a numeric ID for the state_key if there is one present.
	// Otherwise set the numeric ID for the state_key to 0.
	if eventStateKey != nil {
		if eventStateKeyNID, err = d.assignStateKeyNID(*eventStateKey); err != nil {
			return err
		}
	}

	if eventNID, err = d.statements.insertEvent(
		roomNID,
		eventTypeNID,
		eventStateKeyNID,
		event.EventID(),
		event.EventReference().EventSHA256,
	); err != nil {
		return err
	}

	return d.statements.insertEventJSON(eventNID, event.JSON())
}

func (d *Database) assignRoomNID(roomID string) (int64, error) {
	// Check if we already have a numeric ID in the database.
	roomNID, err := d.statements.selectRoomNID(roomID)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		return d.statements.insertRoomNID(roomID)
	}
	if err != nil {
		return 0, err
	}
	return roomNID, nil
}

func (d *Database) assignEventTypeNID(eventType string) (int64, error) {
	// Check if we already have a numeric ID in the database.
	eventTypeNID, err := d.statements.selectEventTypeNID(eventType)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		return d.statements.insertEventTypeNID(eventType)
	}
	if err != nil {
		return 0, err
	}
	return eventTypeNID, nil
}

func (d *Database) assignStateKeyNID(eventStateKey string) (int64, error) {
	// Check if we already have a numeric ID in the database.
	eventStateKeyNID, err := d.statements.selectEventStateKeyNID(eventStateKey)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		return d.statements.insertEventStateKeyNID(eventStateKey)
	}
	if err != nil {
		return 0, err
	}
	return eventStateKeyNID, nil
}
