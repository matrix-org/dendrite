package tables

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type Invites interface {
	InsertInviteEvent(ctx context.Context, txn *sql.Tx, inviteEvent gomatrixserverlib.HeaderedEvent) (streamPos types.StreamPosition, err error)
	DeleteInviteEvent(ctx context.Context, inviteEventID string) error
	SelectInviteEventsInRange(ctx context.Context, txn *sql.Tx, targetUserID string, startPos, endPos types.StreamPosition) (map[string]gomatrixserverlib.HeaderedEvent, error)
	SelectMaxInviteID(ctx context.Context, txn *sql.Tx) (id int64, err error)
}
