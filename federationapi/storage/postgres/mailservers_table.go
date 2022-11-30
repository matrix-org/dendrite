// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package postgres

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

const mailserversSchema = `
CREATE TABLE IF NOT EXISTS federationsender_mailservers (
    -- The destination server name
	server_name TEXT NOT NULL,
	-- The mailserver name for a given destination
	mailserver_name TEXT NOT NULL,
	UNIQUE (server_name, mailserver_name)
);

CREATE INDEX IF NOT EXISTS federationsender_mailservers_server_name_idx
    ON federationsender_mailservers (server_name);
`

const insertMailserversSQL = "" +
	"INSERT INTO federationsender_mailservers (server_name, mailserver_name) VALUES ($1, $2)" +
	" ON CONFLICT DO NOTHING"

const selectMailserversSQL = "" +
	"SELECT mailserver_name FROM federationsender_mailservers WHERE server_name = $1"

const deleteMailserversSQL = "" +
	"DELETE FROM federationsender_mailservers WHERE server_name = $1 AND mailserver_name IN ($2)"

const deleteAllMailserversSQL = "" +
	"DELETE FROM federationsender_mailservers WHERE server_name = $1"

type mailserversStatements struct {
	db                       *sql.DB
	insertMailserversStmt    *sql.Stmt
	selectMailserversStmt    *sql.Stmt
	deleteMailserversStmt    *sql.Stmt
	deleteAllMailserversStmt *sql.Stmt
}

func NewPostgresMailserversTable(db *sql.DB) (s *mailserversStatements, err error) {
	s = &mailserversStatements{
		db: db,
	}
	_, err = db.Exec(mailserversSchema)
	if err != nil {
		return
	}

	if s.insertMailserversStmt, err = db.Prepare(insertMailserversSQL); err != nil {
		return
	}
	if s.selectMailserversStmt, err = db.Prepare(selectMailserversSQL); err != nil {
		return
	}
	if s.deleteMailserversStmt, err = db.Prepare(deleteMailserversSQL); err != nil {
		return
	}
	if s.deleteAllMailserversStmt, err = db.Prepare(deleteAllMailserversSQL); err != nil {
		return
	}
	return
}

func (s *mailserversStatements) InsertMailservers(
	ctx context.Context,
	txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
	mailservers []gomatrixserverlib.ServerName,
) error {
	for _, mailserver := range mailservers {
		stmt := sqlutil.TxStmt(txn, s.insertMailserversStmt)
		if _, err := stmt.ExecContext(ctx, serverName, mailserver); err != nil {
			return err
		}
	}
	return nil
}

func (s *mailserversStatements) SelectMailservers(
	ctx context.Context,
	txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
) ([]gomatrixserverlib.ServerName, error) {
	stmt := sqlutil.TxStmt(txn, s.selectMailserversStmt)
	rows, err := stmt.QueryContext(ctx, serverName)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectMailservers: rows.close() failed")

	var result []gomatrixserverlib.ServerName
	for rows.Next() {
		var mailserver string
		if err = rows.Scan(&mailserver); err != nil {
			return nil, err
		}
		result = append(result, gomatrixserverlib.ServerName(mailserver))
	}
	return result, nil
}

func (s *mailserversStatements) DeleteMailservers(
	ctx context.Context,
	txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
	mailservers []gomatrixserverlib.ServerName,
) error {
	for _, mailserver := range mailservers {
		stmt := sqlutil.TxStmt(txn, s.deleteMailserversStmt)
		if _, err := stmt.ExecContext(ctx, serverName, mailserver); err != nil {
			return err
		}
	}
	return nil
}

func (s *mailserversStatements) DeleteAllMailservers(
	ctx context.Context,
	txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteAllMailserversStmt)
	if _, err := stmt.ExecContext(ctx, serverName); err != nil {
		return err
	}
	return nil
}
