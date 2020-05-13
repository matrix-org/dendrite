package shared

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// Database is a temporary struct until we have made syncserver.go the same for both pq/sqlite
// For now this contains the shared functions
type Database struct {
	DB      *sql.DB
	Invites tables.Invites
}

func (d *Database) AddInviteEvent(
	ctx context.Context, inviteEvent gomatrixserverlib.HeaderedEvent,
) (sp types.StreamPosition, err error) {
	err = common.WithTransaction(d.DB, func(txn *sql.Tx) error {
		sp, err = d.Invites.InsertInviteEvent(ctx, txn, inviteEvent)
		return err
	})
	return
}

func (d *Database) RetireInviteEvent(
	ctx context.Context, inviteEventID string,
) error {
	// TODO: Record that invite has been retired in a stream so that we can
	// notify the user in an incremental sync.
	err := d.Invites.DeleteInviteEvent(ctx, inviteEventID)
	return err
}
