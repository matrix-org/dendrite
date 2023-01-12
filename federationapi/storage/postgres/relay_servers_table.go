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

const relayServersSchema = `
CREATE TABLE IF NOT EXISTS federationsender_relay_servers (
	-- The destination server name
	server_name TEXT NOT NULL,
	-- The relay server name for a given destination
	relay_server_name TEXT NOT NULL,
	UNIQUE (server_name, relay_server_name)
);

CREATE INDEX IF NOT EXISTS federationsender_relay_servers_server_name_idx
	ON federationsender_relay_servers (server_name);
`

const insertRelayServersSQL = "" +
	"INSERT INTO federationsender_relay_servers (server_name, relay_server_name) VALUES ($1, $2)" +
	" ON CONFLICT DO NOTHING"

const selectRelayServersSQL = "" +
	"SELECT relay_server_name FROM federationsender_relay_servers WHERE server_name = $1"

const deleteRelayServersSQL = "" +
	"DELETE FROM federationsender_relay_servers WHERE server_name = $1 AND relay_server_name IN ($2)"

const deleteAllRelayServersSQL = "" +
	"DELETE FROM federationsender_relay_servers WHERE server_name = $1"

type relayServersStatements struct {
	db                        *sql.DB
	insertRelayServersStmt    *sql.Stmt
	selectRelayServersStmt    *sql.Stmt
	deleteRelayServersStmt    *sql.Stmt
	deleteAllRelayServersStmt *sql.Stmt
}

func NewPostgresRelayServersTable(db *sql.DB) (s *relayServersStatements, err error) {
	s = &relayServersStatements{
		db: db,
	}
	_, err = db.Exec(relayServersSchema)
	if err != nil {
		return
	}

	return s, sqlutil.StatementList{
		{&s.insertRelayServersStmt, insertRelayServersSQL},
		{&s.selectRelayServersStmt, selectRelayServersSQL},
		{&s.deleteRelayServersStmt, deleteRelayServersSQL},
		{&s.deleteAllRelayServersStmt, deleteAllRelayServersSQL},
	}.Prepare(db)
}

func (s *relayServersStatements) InsertRelayServers(
	ctx context.Context,
	txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
	relayServers []gomatrixserverlib.ServerName,
) error {
	for _, relayServer := range relayServers {
		stmt := sqlutil.TxStmt(txn, s.insertRelayServersStmt)
		if _, err := stmt.ExecContext(ctx, serverName, relayServer); err != nil {
			return err
		}
	}
	return nil
}

func (s *relayServersStatements) SelectRelayServers(
	ctx context.Context,
	txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
) ([]gomatrixserverlib.ServerName, error) {
	stmt := sqlutil.TxStmt(txn, s.selectRelayServersStmt)
	rows, err := stmt.QueryContext(ctx, serverName)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectRelayServers: rows.close() failed")

	var result []gomatrixserverlib.ServerName
	for rows.Next() {
		var relayServer string
		if err = rows.Scan(&relayServer); err != nil {
			return nil, err
		}
		result = append(result, gomatrixserverlib.ServerName(relayServer))
	}
	return result, nil
}

func (s *relayServersStatements) DeleteRelayServers(
	ctx context.Context,
	txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
	relayServers []gomatrixserverlib.ServerName,
) error {
	for _, relayServer := range relayServers {
		stmt := sqlutil.TxStmt(txn, s.deleteRelayServersStmt)
		if _, err := stmt.ExecContext(ctx, serverName, relayServer); err != nil {
			return err
		}
	}
	return nil
}

func (s *relayServersStatements) DeleteAllRelayServers(
	ctx context.Context,
	txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteAllRelayServersStmt)
	if _, err := stmt.ExecContext(ctx, serverName); err != nil {
		return err
	}
	return nil
}
