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

package sqlite3

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

const blacklistSchema = `
CREATE TABLE IF NOT EXISTS federationsender_blacklist (
    -- The blacklisted server name
	server_name TEXT NOT NULL,
	UNIQUE (server_name)
);
`

const insertBlacklistSQL = "" +
	"INSERT INTO federationsender_blacklist (server_name) VALUES ($1)" +
	" ON CONFLICT DO NOTHING"

const selectBlacklistSQL = "" +
	"SELECT server_name FROM federationsender_blacklist WHERE server_name = $1"

const deleteBlacklistSQL = "" +
	"DELETE FROM federationsender_blacklist WHERE server_name = $1"

type blacklistStatements struct {
	db                  *sql.DB
	writer              *sqlutil.TransactionWriter
	insertBlacklistStmt *sql.Stmt
	selectBlacklistStmt *sql.Stmt
	deleteBlacklistStmt *sql.Stmt
}

func NewSQLiteBlacklistTable(db *sql.DB) (s *blacklistStatements, err error) {
	s = &blacklistStatements{
		db:     db,
		writer: sqlutil.NewTransactionWriter(),
	}
	_, err = db.Exec(blacklistSchema)
	if err != nil {
		return
	}

	if s.insertBlacklistStmt, err = db.Prepare(insertBlacklistSQL); err != nil {
		return
	}
	if s.selectBlacklistStmt, err = db.Prepare(selectBlacklistSQL); err != nil {
		return
	}
	if s.deleteBlacklistStmt, err = db.Prepare(deleteBlacklistSQL); err != nil {
		return
	}
	return
}

// insertRoom inserts the room if it didn't already exist.
// If the room didn't exist then last_event_id is set to the empty string.
func (s *blacklistStatements) InsertBlacklist(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName,
) error {
	return s.writer.Do(s.db, txn, func(txn *sql.Tx) error {
		stmt := sqlutil.TxStmt(txn, s.insertBlacklistStmt)
		_, err := stmt.ExecContext(ctx, serverName)
		return err
	})
}

// selectRoomForUpdate locks the row for the room and returns the last_event_id.
// The row must already exist in the table. Callers can ensure that the row
// exists by calling insertRoom first.
func (s *blacklistStatements) SelectBlacklist(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName,
) (bool, error) {
	stmt := sqlutil.TxStmt(txn, s.selectBlacklistStmt)
	res, err := stmt.QueryContext(ctx, serverName)
	if err != nil {
		return false, err
	}
	defer res.Close() // nolint:errcheck
	// The query will return the server name if the server is blacklisted, and
	// will return no rows if not. By calling Next, we find out if a row was
	// returned or not - we don't care about the value itself.
	return res.Next(), nil
}

// updateRoom updates the last_event_id for the room. selectRoomForUpdate should
// have already been called earlier within the transaction.
func (s *blacklistStatements) DeleteBlacklist(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName,
) error {
	return s.writer.Do(s.db, txn, func(txn *sql.Tx) error {
		stmt := sqlutil.TxStmt(txn, s.deleteBlacklistStmt)
		_, err := stmt.ExecContext(ctx, serverName)
		return err
	})
}
