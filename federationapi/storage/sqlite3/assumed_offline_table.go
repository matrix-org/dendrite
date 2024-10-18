// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package sqlite3

import (
	"context"
	"database/sql"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

const assumedOfflineSchema = `
CREATE TABLE IF NOT EXISTS federationsender_assumed_offline(
    -- The assumed offline server name
	server_name TEXT PRIMARY KEY NOT NULL
);
`

const insertAssumedOfflineSQL = "" +
	"INSERT INTO federationsender_assumed_offline (server_name) VALUES ($1)" +
	" ON CONFLICT DO NOTHING"

const selectAssumedOfflineSQL = "" +
	"SELECT server_name FROM federationsender_assumed_offline WHERE server_name = $1"

const deleteAssumedOfflineSQL = "" +
	"DELETE FROM federationsender_assumed_offline WHERE server_name = $1"

const deleteAllAssumedOfflineSQL = "" +
	"DELETE FROM federationsender_assumed_offline"

type assumedOfflineStatements struct {
	db                          *sql.DB
	insertAssumedOfflineStmt    *sql.Stmt
	selectAssumedOfflineStmt    *sql.Stmt
	deleteAssumedOfflineStmt    *sql.Stmt
	deleteAllAssumedOfflineStmt *sql.Stmt
}

func NewSQLiteAssumedOfflineTable(db *sql.DB) (s *assumedOfflineStatements, err error) {
	s = &assumedOfflineStatements{
		db: db,
	}
	_, err = db.Exec(assumedOfflineSchema)
	if err != nil {
		return
	}

	return s, sqlutil.StatementList{
		{&s.insertAssumedOfflineStmt, insertAssumedOfflineSQL},
		{&s.selectAssumedOfflineStmt, selectAssumedOfflineSQL},
		{&s.deleteAssumedOfflineStmt, deleteAssumedOfflineSQL},
		{&s.deleteAllAssumedOfflineStmt, deleteAllAssumedOfflineSQL},
	}.Prepare(db)
}

func (s *assumedOfflineStatements) InsertAssumedOffline(
	ctx context.Context, txn *sql.Tx, serverName spec.ServerName,
) error {
	stmt := sqlutil.TxStmt(txn, s.insertAssumedOfflineStmt)
	_, err := stmt.ExecContext(ctx, serverName)
	return err
}

func (s *assumedOfflineStatements) SelectAssumedOffline(
	ctx context.Context, txn *sql.Tx, serverName spec.ServerName,
) (bool, error) {
	stmt := sqlutil.TxStmt(txn, s.selectAssumedOfflineStmt)
	res, err := stmt.QueryContext(ctx, serverName)
	if err != nil {
		return false, err
	}
	defer res.Close() // nolint:errcheck
	// The query will return the server name if the server is assume offline, and
	// will return no rows if not. By calling Next, we find out if a row was
	// returned or not - we don't care about the value itself.
	return res.Next(), nil
}

func (s *assumedOfflineStatements) DeleteAssumedOffline(
	ctx context.Context, txn *sql.Tx, serverName spec.ServerName,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteAssumedOfflineStmt)
	_, err := stmt.ExecContext(ctx, serverName)
	return err
}

func (s *assumedOfflineStatements) DeleteAllAssumedOffline(
	ctx context.Context, txn *sql.Tx,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteAllAssumedOfflineStmt)
	_, err := stmt.ExecContext(ctx)
	return err
}
