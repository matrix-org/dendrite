package sqlutil

import "database/sql"

// The Writer interface is designed to solve the problem of how
// to handle database writes for database engines that don't allow
// concurrent writes, e.g. SQLite.
//
// The interface has a single Do function which takes an optional
// database parameter, an optional transaction parameter and a
// required function parameter. The Writer will call the function
// provided when it is safe to do so, optionally providing a
// transaction to use.
//
// Depending on the combination of parameters provided, the Writer
// will behave in one of three ways:
//
// 1. `db` provided, `txn` provided:
//
// The Writer will call f() when it is safe to do so. The supplied
// "txn" will ALWAYS be passed through to f(). Use this when you
// already have a transaction open.
//
// 2. `db` provided, `txn` not provided (nil):
//
// The Writer will open a new transaction on the provided database
// and then will call f() when it is safe to do so. The new
// transaction will ALWAYS be passed through to f(). Use this if
// you plan to perform more than one SQL query within f().
//
// 3. `db` not provided (nil), `txn` not provided (nil):
//
// The Writer will call f() when it is safe to do so, but will
// not make any attempt to open a new database transaction or to
// pass through an existing one. The "txn" parameter within f()
// will ALWAYS be nil in this mode. This is useful if you just
// want to perform a single query on an already-prepared statement
// without the overhead of opening a new transaction to do it in.
//
// You MUST take particular care not to call Do() from within f()
// on the same Writer, or it will likely result in a deadlock.
type Writer interface {
	// Queue up one or more database write operations within the
	// provided function to be executed when it is safe to do so.
	Do(db *sql.DB, txn *sql.Tx, f func(txn *sql.Tx) error) error
}
