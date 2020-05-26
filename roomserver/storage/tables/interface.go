package tables

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type EventJSONPair struct {
	EventNID  types.EventNID
	EventJSON []byte
}

type EventJSON interface {
	InsertEventJSON(ctx context.Context, tx *sql.Tx, eventNID types.EventNID, eventJSON []byte) error
	BulkSelectEventJSON(ctx context.Context, eventNIDs []types.EventNID) ([]EventJSONPair, error)
}

type EventTypes interface {
	InsertEventTypeNID(ctx context.Context, tx *sql.Tx, eventType string) (types.EventTypeNID, error)
	SelectEventTypeNID(ctx context.Context, tx *sql.Tx, eventType string) (types.EventTypeNID, error)
	BulkSelectEventTypeNID(ctx context.Context, eventTypes []string) (map[string]types.EventTypeNID, error)
}

type EventStateKeys interface {
	InsertEventStateKeyNID(ctx context.Context, txn *sql.Tx, eventStateKey string) (types.EventStateKeyNID, error)
	SelectEventStateKeyNID(ctx context.Context, txn *sql.Tx, eventStateKey string) (types.EventStateKeyNID, error)
	BulkSelectEventStateKeyNID(ctx context.Context, eventStateKeys []string) (map[string]types.EventStateKeyNID, error)
	BulkSelectEventStateKey(ctx context.Context, eventStateKeyNIDs []types.EventStateKeyNID) (map[types.EventStateKeyNID]string, error)
}

type Events interface {
	InsertEvent(c context.Context, txn *sql.Tx, i types.RoomNID, j types.EventTypeNID, k types.EventStateKeyNID, eventID string, referenceSHA256 []byte, authEventNIDs []types.EventNID, depth int64) (types.EventNID, types.StateSnapshotNID, error)
	SelectEvent(ctx context.Context, txn *sql.Tx, eventID string) (types.EventNID, types.StateSnapshotNID, error)
	// bulkSelectStateEventByID lookups a list of state events by event ID.
	// If any of the requested events are missing from the database it returns a types.MissingEventError
	BulkSelectStateEventByID(ctx context.Context, eventIDs []string) ([]types.StateEntry, error)
	// BulkSelectStateAtEventByID lookups the state at a list of events by event ID.
	// If any of the requested events are missing from the database it returns a types.MissingEventError.
	// If we do not have the state for any of the requested events it returns a types.MissingEventError.
	BulkSelectStateAtEventByID(ctx context.Context, eventIDs []string) ([]types.StateAtEvent, error)
	UpdateEventState(ctx context.Context, eventNID types.EventNID, stateNID types.StateSnapshotNID) error
	SelectEventSentToOutput(ctx context.Context, txn *sql.Tx, eventNID types.EventNID) (sentToOutput bool, err error)
	UpdateEventSentToOutput(ctx context.Context, txn *sql.Tx, eventNID types.EventNID) error
	SelectEventID(ctx context.Context, txn *sql.Tx, eventNID types.EventNID) (eventID string, err error)
	BulkSelectStateAtEventAndReference(ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID) ([]types.StateAtEventAndReference, error)
	BulkSelectEventReference(ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID) ([]gomatrixserverlib.EventReference, error)
	// BulkSelectEventID returns a map from numeric event ID to string event ID.
	BulkSelectEventID(ctx context.Context, eventNIDs []types.EventNID) (map[types.EventNID]string, error)
	// BulkSelectEventNIDs returns a map from string event ID to numeric event ID.
	// If an event ID is not in the database then it is omitted from the map.
	BulkSelectEventNID(ctx context.Context, eventIDs []string) (map[string]types.EventNID, error)
	SelectMaxEventDepth(ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID) (int64, error)
	SelectRoomNIDForEventNID(ctx context.Context, txn *sql.Tx, eventNID types.EventNID) (roomNID types.RoomNID, err error)
}
