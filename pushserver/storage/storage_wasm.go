package storage

import (
	"fmt"

	"github.com/matrix-org/dendrite/pushserver/storage/sqlite3"
	"github.com/matrix-org/dendrite/setup/config"
)

// NewDatabase opens a new database
func Open(dbProperties *config.DatabaseOptions) (Database, error) {
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		return sqlite3.Open(dbProperties)
	case dbProperties.ConnectionString.IsPostgres():
		return nil, fmt.Errorf("can't use Postgres implementation")
	default:
		return nil, fmt.Errorf("unexpected database type")
	}
}
