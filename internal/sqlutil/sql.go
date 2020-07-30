// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlutil

import (
	"database/sql"
	"errors"
	"fmt"
	"runtime"

	"go.uber.org/atomic"
)

// ErrUserExists is returned if a username already exists in the database.
var ErrUserExists = errors.New("Username already exists")

// A Transaction is something that can be committed or rolledback.
type Transaction interface {
	// Commit the transaction
	Commit() error
	// Rollback the transaction.
	Rollback() error
}

// EndTransaction ends a transaction.
// If the transaction succeeded then it is committed, otherwise it is rolledback.
// You MUST check the error returned from this function to be sure that the transaction
// was applied correctly. For example, 'database is locked' errors in sqlite will happen here.
func EndTransaction(txn Transaction, succeeded *bool) error {
	if *succeeded {
		return txn.Commit() // nolint: errcheck
	} else {
		return txn.Rollback() // nolint: errcheck
	}
}

// WithTransaction runs a block of code passing in an SQL transaction
// If the code returns an error or panics then the transactions is rolledback
// Otherwise the transaction is committed.
func WithTransaction(db *sql.DB, fn func(txn *sql.Tx) error) (err error) {
	txn, err := db.Begin()
	if err != nil {
		return
	}
	succeeded := false
	defer func() {
		err2 := EndTransaction(txn, &succeeded)
		if err == nil && err2 != nil { // failed to commit/rollback
			err = err2
		}
	}()

	err = fn(txn)
	if err != nil {
		return
	}

	succeeded = true
	return
}

// TxStmt wraps an SQL stmt inside an optional transaction.
// If the transaction is nil then it returns the original statement that will
// run outside of a transaction.
// Otherwise returns a copy of the statement that will run inside the transaction.
func TxStmt(transaction *sql.Tx, statement *sql.Stmt) *sql.Stmt {
	if transaction != nil {
		statement = transaction.Stmt(statement)
	}
	return statement
}

// Hack of the century
func QueryVariadic(count int) string {
	return QueryVariadicOffset(count, 0)
}

func QueryVariadicOffset(count, offset int) string {
	str := "("
	for i := 0; i < count; i++ {
		str += fmt.Sprintf("$%d", i+offset+1)
		if i < (count - 1) {
			str += ", "
		}
	}
	str += ")"
	return str
}

func SQLiteDriverName() string {
	if runtime.GOOS == "js" {
		return "sqlite3_js"
	}
	return "sqlite3"
}

// TransactionWriter allows queuing database writes so that you don't
// contend on database locks in, e.g. SQLite. Only one task will run
// at a time on a given TransactionWriter.
type TransactionWriter struct {
	running atomic.Bool
	todo    chan transactionWriterTask
}

func NewTransactionWriter() *TransactionWriter {
	return &TransactionWriter{
		todo: make(chan transactionWriterTask),
	}
}

// transactionWriterTask represents a specific task.
type transactionWriterTask struct {
	db   *sql.DB
	txn  *sql.Tx
	f    func(txn *sql.Tx) error
	wait chan error
}

// Do queues a task to be run by a TransactionWriter. The function
// provided will be ran within a transaction as supplied by the
// txn parameter if one is supplied, and if not, will take out a
// new transaction from the database supplied in the database
// parameter. Either way, this will block until the task is done.
func (w *TransactionWriter) Do(db *sql.DB, txn *sql.Tx, f func(txn *sql.Tx) error) error {
	if w.todo == nil {
		return errors.New("not initialised")
	}
	if !w.running.Load() {
		go w.run()
	}
	task := transactionWriterTask{
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
func (w *TransactionWriter) run() {
	if !w.running.CAS(false, true) {
		return
	}
	defer w.running.Store(false)
	for task := range w.todo {
		if task.txn != nil {
			task.wait <- task.f(task.txn)
		} else if task.db != nil {
			task.wait <- WithTransaction(task.db, func(txn *sql.Tx) error {
				return task.f(txn)
			})
		} else {
			panic("expected database or transaction but got neither")
		}
		close(task.wait)
	}
}
