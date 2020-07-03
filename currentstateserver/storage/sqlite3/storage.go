package sqlite3

import (
	"database/sql"

	"github.com/matrix-org/dendrite/currentstateserver/storage/shared"
	"github.com/matrix-org/dendrite/internal/sqlutil"
)

type Database struct {
	shared.Database
	db *sql.DB
	sqlutil.PartitionOffsetStatements
}

// NewDatabase creates a new sync server database
// nolint: gocyclo
func NewDatabase(dataSourceName string) (*Database, error) {
	var d Database
	cs, err := sqlutil.ParseFileURI(dataSourceName)
	if err != nil {
		return nil, err
	}
	if d.db, err = sqlutil.Open(sqlutil.SQLiteDriverName(), cs, nil); err != nil {
		return nil, err
	}
	if err = d.PartitionOffsetStatements.Prepare(d.db, "currentstate"); err != nil {
		return nil, err
	}
	currRoomState, err := NewSqliteCurrentRoomStateTable(d.db)
	if err != nil {
		return nil, err
	}
	d.Database = shared.Database{
		DB:               d.db,
		CurrentRoomState: currRoomState,
	}
	return &d, nil
}
