package sqlutil

import (
	"database/sql"
)

// DummyWriter implements sqlutil.Writer.
// The DummyWriter is designed to allow reuse of the sqlutil.Writer
// interface but, unlike ExclusiveWriter, it will not guarantee
// writer exclusivity. This is fine in PostgreSQL where overlapping
// transactions and writes are acceptable.
type DummyWriter struct {
}

// NewDummyWriter returns a new dummy writer.
func NewDummyWriter() Writer {
	return &DummyWriter{}
}

func (w *DummyWriter) Do(db *sql.DB, txn *sql.Tx, f func(txn *sql.Tx) error) error {
	if db != nil && txn == nil {
		return WithTransaction(db, func(txn *sql.Tx) error {
			return f(txn)
		})
	} else {
		return f(txn)
	}
}
