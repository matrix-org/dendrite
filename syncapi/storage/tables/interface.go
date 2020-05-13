package tables

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type AccountData interface {
	InsertAccountData(ctx context.Context, txn *sql.Tx, userID, roomID, dataType string) (pos types.StreamPosition, err error)
	SelectAccountDataInRange(ctx context.Context, userID string, oldPos, newPos types.StreamPosition, accountDataEventFilter *gomatrixserverlib.EventFilter) (data map[string][]string, err error)
	SelectMaxAccountDataID(ctx context.Context, txn *sql.Tx) (id int64, err error)
}

type Invites interface {
	InsertInviteEvent(ctx context.Context, txn *sql.Tx, inviteEvent gomatrixserverlib.HeaderedEvent) (streamPos types.StreamPosition, err error)
	DeleteInviteEvent(ctx context.Context, inviteEventID string) error
	SelectInviteEventsInRange(ctx context.Context, txn *sql.Tx, targetUserID string, startPos, endPos types.StreamPosition) (map[string]gomatrixserverlib.HeaderedEvent, error)
	SelectMaxInviteID(ctx context.Context, txn *sql.Tx) (id int64, err error)
}

type Events interface {
	SelectStateInRange(ctx context.Context, txn *sql.Tx, oldPos, newPos types.StreamPosition, stateFilter *gomatrixserverlib.StateFilter) (map[string]map[string]bool, map[string]types.StreamEvent, error)
	SelectMaxEventID(ctx context.Context, txn *sql.Tx) (id int64, err error)
	InsertEvent(ctx context.Context, txn *sql.Tx, event *gomatrixserverlib.HeaderedEvent, addState, removeState []string, transactionID *api.TransactionID, excludeFromSync bool) (streamPos types.StreamPosition, err error)
	SelectRecentEvents(ctx context.Context, txn *sql.Tx, roomID string, fromPos, toPos types.StreamPosition, limit int, chronologicalOrder bool, onlySyncEvents bool) ([]types.StreamEvent, error)
	SelectEarlyEvents(ctx context.Context, txn *sql.Tx, roomID string, fromPos, toPos types.StreamPosition, limit int) ([]types.StreamEvent, error)
	SelectEvents(ctx context.Context, txn *sql.Tx, eventIDs []string) ([]types.StreamEvent, error)
}
