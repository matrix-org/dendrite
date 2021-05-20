package cosmosdbutil

import (
	"database/sql"
	"errors"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"go.uber.org/atomic"
)

// ExclusiveWriter implements sqlutil.Writer.
// Allows non-SQL DBs to still use the same infrastructure
// as Matrix assumes SQL TXN
type ExclusiveWriterFake struct {
	running atomic.Bool
	todo    chan transactionWriterTaskFake
}

func NewExclusiveWriterFake() sqlutil.Writer {
	return &ExclusiveWriterFake{
		todo: make(chan transactionWriterTaskFake),
	}
}

// transactionWriterTask represents a specific task.
type transactionWriterTaskFake struct {
	db   *sql.DB
	txn  *sql.Tx
	f    func(txn *sql.Tx) error
	wait chan error
}

func (w *ExclusiveWriterFake) Do(db *sql.DB, txn *sql.Tx, f func(txn *sql.Tx) error) error {
	if w.todo == nil {
		return errors.New("not initialised")
	}
	if !w.running.Load() {
		go w.run()
	}
	task := transactionWriterTaskFake{
		db:   db,
		txn:  txn,
		f:    f,
		wait: make(chan error, 1),
	}
	w.todo <- task
	return <-task.wait
}

// run processes the tasks for a given transaction writer. Only one
// of these goroutines will run at a time. A transaction will be
// opened using the database object from the task and then this will
// be passed as a parameter to the task function.
func (w *ExclusiveWriterFake) run() {
	if !w.running.CAS(false, true) {
		return
	}
	// if tracingEnabled {
	// 	gid := goid()
	// 	goidToWriter.Store(gid, w)
	// 	defer goidToWriter.Delete(gid)
	// }

	defer w.running.Store(false)
	for task := range w.todo {
		if task.db != nil && task.txn != nil {
			task.wait <- task.f(task.txn)
			// } else if task.db != nil && task.txn == nil {
			// 	task.wait <- WithTransaction(task.db, func(txn *sql.Tx) error {
			// 		return task.f(txn)
			// 	})
		} else {
			task.wait <- task.f(nil)
		}
		close(task.wait)
	}
}
