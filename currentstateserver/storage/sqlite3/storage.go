package sqlite3

import (
	"database/sql"

	"github.com/matrix-org/dendrite/currentstateserver/storage/shared"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/sqlutil"
)

type Database struct {
	shared.Database
	db *sql.DB
	sqlutil.PartitionOffsetStatements
}

// NewDatabase creates a new sync server database
// nolint: gocyclo
func NewDatabase(dbProperties *config.DatabaseOptions) (*Database, error) {
	var d Database
	var err error
	if d.db, err = sqlutil.Open(dbProperties); err != nil {
		return nil, err
	}
	writer := sqlutil.NewTransactionWriter()
	if err = d.PartitionOffsetStatements.Prepare(d.db, "currentstate"); err != nil {
		return nil, err
	}
	currRoomState, err := NewSqliteCurrentRoomStateTable(d.db, writer)
	if err != nil {
		return nil, err
	}
	d.Database = shared.Database{
		DB:               d.db,
		CurrentRoomState: currRoomState,
	}
	return &d, nil
}
