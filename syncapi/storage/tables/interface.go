package tables

import (
	"context"
	"database/sql"

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
