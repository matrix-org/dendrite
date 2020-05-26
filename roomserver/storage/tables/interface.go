package tables

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/roomserver/types"
)

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
