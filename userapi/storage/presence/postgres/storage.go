package postgres

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/types"
)

// Database represents an account database
type Database struct {
	db     *sql.DB
	writer sqlutil.Writer
	sqlutil.PartitionOffsetStatements
	presence presenceStatements
}

func NewDatabase(dbProperties *config.DatabaseOptions) (*Database, error) {
	db, err := sqlutil.Open(dbProperties)
	if err != nil {
		return nil, err
	}
	d := &Database{
		db:     db,
		writer: sqlutil.NewDummyWriter(),
	}

	if err = d.presence.execSchema(db); err != nil {
		return nil, err
	}
	if err = d.PartitionOffsetStatements.Prepare(db, d.writer, "presence"); err != nil {
		return nil, err
	}
	if err = d.presence.prepare(db); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Database) UpsertPresence(
	ctx context.Context,
	userID, statusMsg string,
	presence types.PresenceStatus,
	lastActiveTS int64,
) (pos int64, err error) {
	err = sqlutil.WithTransaction(d.db, func(txn *sql.Tx) error {
		pos, err = d.presence.UpsertPresence(ctx, txn, userID, statusMsg, presence, lastActiveTS)
		return err
	})
	return
}

func (d *Database) GetPresenceForUser(ctx context.Context, userID string) (api.OutputPresence, error) {
	return d.presence.GetPresenceForUser(ctx, nil, userID)
}
