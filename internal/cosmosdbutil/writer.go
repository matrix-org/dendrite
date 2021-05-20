package cosmosdbutil

import (
	"database/sql"
)

// The Writer interface is designed to solve the problem of how
// to handle database writes for database engines that don't allow
// concurrent writes, e.g. SQLite.
// 

// Copied for CosmosDB compatibility

type Writer interface {
	Do(db *sql.DB, txn *sql.Tx ,f func(txn *sql.Tx) error) error
}
