// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package sqlutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/matrix-org/util"
)

// ErrUserExists is returned if a username already exists in the database.
var ErrUserExists = errors.New("username already exists")

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
		return txn.Commit()
	} else {
		return txn.Rollback()
	}
}

// EndTransactionWithCheck ends a transaction and overwrites the error pointer if its value was nil.
// If the transaction succeeded then it is committed, otherwise it is rolledback.
// Designed to be used with defer (see EndTransaction otherwise).
func EndTransactionWithCheck(txn Transaction, succeeded *bool, err *error) {
	if e := EndTransaction(txn, succeeded); e != nil && *err == nil {
		*err = e
	}
}

// WithTransaction runs a block of code passing in an SQL transaction
// If the code returns an error or panics then the transactions is rolledback
// Otherwise the transaction is committed.
func WithTransaction(db *sql.DB, fn func(txn *sql.Tx) error) (err error) {
	txn, err := db.Begin()
	if err != nil {
		return fmt.Errorf("sqlutil.WithTransaction.Begin: %w", err)
	}
	succeeded := false
	defer EndTransactionWithCheck(txn, &succeeded, &err)

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

// TxStmtContext behaves similarly to TxStmt, with support for also passing context.
func TxStmtContext(context context.Context, transaction *sql.Tx, statement *sql.Stmt) *sql.Stmt {
	if transaction != nil {
		statement = transaction.StmtContext(context, statement)
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

func minOfInts(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

// QueryProvider defines the interface for querys used by RunLimitedVariablesQuery.
type QueryProvider interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
}

// ExecProvider defines the interface for querys used by RunLimitedVariablesExec.
type ExecProvider interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// SQLite3MaxVariables is the default maximum number of host parameters in a single SQL statement
// SQLlite can handle. See https://www.sqlite.org/limits.html for more information.
const SQLite3MaxVariables = 999

// RunLimitedVariablesQuery split up a query with more variables than the used database can handle in multiple queries.
func RunLimitedVariablesQuery(ctx context.Context, query string, qp QueryProvider, variables []interface{}, limit uint, rowHandler func(*sql.Rows) error) error {
	var start int
	for start < len(variables) {
		n := minOfInts(len(variables)-start, int(limit))
		nextQuery := strings.Replace(query, "($1)", QueryVariadic(n), 1)
		rows, err := qp.QueryContext(ctx, nextQuery, variables[start:start+n]...)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("QueryContext returned an error")
			return err
		}
		err = rowHandler(rows)
		if closeErr := rows.Close(); closeErr != nil {
			util.GetLogger(ctx).WithError(closeErr).Error("RunLimitedVariablesQuery: failed to close rows")
			return err
		}
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("RunLimitedVariablesQuery: rowHandler returned error")
			return err
		}
		start = start + n
	}
	return nil
}

// RunLimitedVariablesExec split up a query with more variables than the used database can handle in multiple queries.
func RunLimitedVariablesExec(ctx context.Context, query string, qp ExecProvider, variables []interface{}, limit uint) error {
	var start int
	for start < len(variables) {
		n := minOfInts(len(variables)-start, int(limit))
		nextQuery := strings.Replace(query, "($1)", QueryVariadic(n), 1)
		_, err := qp.ExecContext(ctx, nextQuery, variables[start:start+n]...)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("ExecContext returned an error")
			return err
		}
		start = start + n
	}
	return nil
}

// StatementList is a list of SQL statements to prepare and a pointer to where to store the resulting prepared statement.
type StatementList []struct {
	Statement **sql.Stmt
	SQL       string
}

// Prepare the SQL for each statement in the list and assign the result to the prepared statement.
func (s StatementList) Prepare(db *sql.DB) (err error) {
	for _, statement := range s {
		if *statement.Statement, err = db.Prepare(statement.SQL); err != nil {
			err = fmt.Errorf("Error %q while preparing statement: %s", err, statement.SQL)
			return
		}
	}
	return
}
