// +build !wasm

package storage

import (
	"fmt"

	"github.com/matrix-org/dendrite/pushserver/storage/postgres"
	"github.com/matrix-org/dendrite/pushserver/storage/sqlite3"
	"github.com/matrix-org/dendrite/setup/config"
)

// Open opens a database connection.
func Open(dbProperties *config.DatabaseOptions) (Database, error) {
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		return sqlite3.Open(dbProperties)
	case dbProperties.ConnectionString.IsPostgres():
		return postgres.Open(dbProperties)
	default:
		return nil, fmt.Errorf("unexpected database type")
	}
}
