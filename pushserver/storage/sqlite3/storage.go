package sqlite3

import (
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/pushserver/storage/shared"
	"github.com/matrix-org/dendrite/setup/config"
)

type Database struct {
	shared.Database
	sqlutil.PartitionOffsetStatements
}

func Open(dbProperties *config.DatabaseOptions) (*Database, error) {
	var d Database
	var err error
	if d.DB, err = sqlutil.Open(dbProperties); err != nil {
		return nil, fmt.Errorf("sqlutil.Open: %w", err)
	}
	d.Writer = sqlutil.NewExclusiveWriter()

	if err = d.PartitionOffsetStatements.Prepare(d.DB, d.Writer, "pushserver"); err != nil {
		return nil, err
	}

	// Create the tables.

	return &d, nil
}
