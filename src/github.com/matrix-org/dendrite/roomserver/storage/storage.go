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
func (d *Database) StoreEvent(event gomatrixserverlib.Event, authEventNIDs []types.EventNID) (types.RoomNID, types.StateAtEvent, error) {
	var (
		roomNID          types.RoomNID
		eventTypeNID     types.EventTypeNID
		eventStateKeyNID types.EventStateKeyNID
		eventNID         types.EventNID
		stateNID         types.StateSnapshotNID
		err              error
	)

	if roomNID, err = d.assignRoomNID(event.RoomID()); err != nil {
		return 0, types.StateAtEvent{}, err
	}

	if eventTypeNID, err = d.assignEventTypeNID(event.Type()); err != nil {
		return 0, types.StateAtEvent{}, err
	}

	eventStateKey := event.StateKey()
	// Assigned a numeric ID for the state_key if there is one present.
	// Otherwise set the numeric ID for the state_key to 0.
	if eventStateKey != nil {
		if eventStateKeyNID, err = d.assignStateKeyNID(*eventStateKey); err != nil {
			return 0, types.StateAtEvent{}, err
		}
	}

	if eventNID, stateNID, err = d.statements.insertEvent(
		roomNID,
		eventTypeNID,
		eventStateKeyNID,
		event.EventID(),
		event.EventReference().EventSHA256,
		authEventNIDs,
	); err != nil {
		return 0, types.StateAtEvent{}, err
	}

	if err = d.statements.insertEventJSON(eventNID, event.JSON()); err != nil {
		return 0, types.StateAtEvent{}, err
	}

	return roomNID, types.StateAtEvent{
		BeforeStateSnapshotNID: stateNID,
		StateEntry: types.StateEntry{
			StateKeyTuple: types.StateKeyTuple{
				EventTypeNID:     eventTypeNID,
				EventStateKeyNID: eventStateKeyNID,
			},
			EventNID: eventNID,
		},
	}, nil
}

func (d *Database) assignRoomNID(roomID string) (types.RoomNID, error) {
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

func (d *Database) assignEventTypeNID(eventType string) (types.EventTypeNID, error) {
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

func (d *Database) assignStateKeyNID(eventStateKey string) (types.EventStateKeyNID, error) {
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

// StateEntriesForEventIDs implements input.EventDatabase
func (d *Database) StateEntriesForEventIDs(eventIDs []string) ([]types.StateEntry, error) {
	return d.statements.bulkSelectStateEventByID(eventIDs)
}

// EventStateKeyNIDs implements input.EventDatabase
func (d *Database) EventStateKeyNIDs(eventStateKeys []string) (map[string]types.EventStateKeyNID, error) {
	return d.statements.bulkSelectEventStateKeyNID(eventStateKeys)
}

// Events implements input.EventDatabase
func (d *Database) Events(eventNIDs []types.EventNID) ([]types.Event, error) {
	eventJSONs, err := d.statements.bulkSelectEventJSON(eventNIDs)
	if err != nil {
		return nil, err
	}
	results := make([]types.Event, len(eventJSONs))
	for i, eventJSON := range eventJSONs {
		result := &results[i]
		result.EventNID = eventJSON.EventNID
		// TODO: Use NewEventFromTrustedJSON for efficiency
		result.Event, err = gomatrixserverlib.NewEventFromUntrustedJSON(eventJSON.EventJSON)
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

// AddState implements input.EventDatabase
func (d *Database) AddState(roomNID types.RoomNID, stateBlockNIDs []types.StateBlockNID, state []types.StateEntry) (types.StateSnapshotNID, error) {
	if len(state) > 0 {
		stateBlockNID, err := d.statements.selectNextStateBlockNID()
		if err != nil {
			return 0, err
		}
		if err = d.statements.bulkInsertStateData(stateBlockNID, state); err != nil {
			return 0, err
		}
		stateBlockNIDs = append(stateBlockNIDs[:len(stateBlockNIDs):len(stateBlockNIDs)], stateBlockNID)
	}

	return d.statements.insertState(roomNID, stateBlockNIDs)
}

// SetState implements input.EventDatabase
func (d *Database) SetState(eventNID types.EventNID, stateNID types.StateSnapshotNID) error {
	return d.statements.updateEventState(eventNID, stateNID)
}

// StateAtEventIDs implements input.EventDatabase
func (d *Database) StateAtEventIDs(eventIDs []string) ([]types.StateAtEvent, error) {
	return d.statements.bulkSelectStateAtEventByID(eventIDs)
}

// StateBlockNIDs implements input.EventDatabase
func (d *Database) StateBlockNIDs(stateNIDs []types.StateSnapshotNID) ([]types.StateBlockNIDList, error) {
	return d.statements.bulkSelectStateBlockNIDs(stateNIDs)
}

// StateEntries implements input.EventDatabase
func (d *Database) StateEntries(stateBlockNIDs []types.StateBlockNID) ([]types.StateEntryList, error) {
	return d.statements.bulkSelectStateDataEntries(stateBlockNIDs)
}

// GetLatestEventsForUpdate implements input.EventDatabase
func (d *Database) GetLatestEventsForUpdate(roomNID types.RoomNID) ([]types.StateAtEventAndReference, types.RoomRecentEventsUpdater, error) {
	txn, err := d.db.Begin()
	if err != nil {
		return nil, nil, err
	}
	eventNIDs, err := d.statements.selectLatestEventsNIDsForUpdate(txn, roomNID)
	if err != nil {
		txn.Rollback()
		return nil, nil, err
	}
	stateAndRefs, err := d.statements.bulkSelectStateAtEventAndReference(txn, eventNIDs)
	if err != nil {
		txn.Rollback()
		return nil, nil, err
	}
	return stateAndRefs, &roomRecentEventsUpdater{txn, d}, nil
}

type roomRecentEventsUpdater struct {
	txn *sql.Tx
	d   *Database
}

func (u *roomRecentEventsUpdater) StorePreviousEvents(eventNID types.EventNID, previousEventReferences []gomatrixserverlib.EventReference) error {
	for _, ref := range previousEventReferences {
		if err := u.d.statements.insertPreviousEvent(u.txn, ref.EventID, ref.EventSHA256, eventNID); err != nil {
			return err
		}
	}
	return nil
}

func (u *roomRecentEventsUpdater) IsReferenced(eventReference gomatrixserverlib.EventReference) (bool, error) {
	err := u.d.statements.selectPreviousEventExists(u.txn, eventReference.EventID, eventReference.EventSHA256)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, err
}

func (u *roomRecentEventsUpdater) SetLatestEvents(roomNID types.RoomNID, latest []types.StateAtEventAndReference) error {
	eventNIDs := make([]types.EventNID, len(latest))
	for i := range latest {
		eventNIDs[i] = latest[i].EventNID
	}
	return u.d.statements.updateLatestEventNIDs(u.txn, roomNID, eventNIDs)
}

func (u *roomRecentEventsUpdater) Commit() error {
	return u.txn.Commit()
}

func (u *roomRecentEventsUpdater) Rollback() error {
	return u.txn.Rollback()
}
