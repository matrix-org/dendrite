// Copyright 2017 Vector Creations Ltd
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

package common

import (
	"database/sql"
	"fmt"
	"runtime"
	"time"
)

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

// DbProperties functions return properties used by database/sql/DB
type DbProperties interface {
	MaxIdleConns() int
	MaxOpenConns() int
	ConnMaxLifetime() time.Duration
}
