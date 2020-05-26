package shared

import (
	"context"

	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
)

type Database struct {
	EventJSON           tables.EventJSON
	EventTypesTable     tables.EventTypes
	EventStateKeysTable tables.EventStateKeys
}

// EventTypeNIDs implements state.RoomStateDatabase
func (d *Database) EventTypeNIDs(
	ctx context.Context, eventTypes []string,
) (map[string]types.EventTypeNID, error) {
	return d.EventTypesTable.BulkSelectEventTypeNID(ctx, eventTypes)
}

// EventStateKeys implements query.RoomserverQueryAPIDatabase
func (d *Database) EventStateKeys(
	ctx context.Context, eventStateKeyNIDs []types.EventStateKeyNID,
) (map[types.EventStateKeyNID]string, error) {
	return d.EventStateKeysTable.BulkSelectEventStateKey(ctx, eventStateKeyNIDs)
}

// EventStateKeyNIDs implements state.RoomStateDatabase
func (d *Database) EventStateKeyNIDs(
	ctx context.Context, eventStateKeys []string,
) (map[string]types.EventStateKeyNID, error) {
	return d.EventStateKeysTable.BulkSelectEventStateKeyNID(ctx, eventStateKeys)
}
