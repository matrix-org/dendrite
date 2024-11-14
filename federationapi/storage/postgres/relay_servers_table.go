// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/lib/pq"
	"github.com/matrix-org/gomatrixserverlib/spec"
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
	"DELETE FROM federationsender_relay_servers WHERE server_name = $1 AND relay_server_name = ANY($2)"

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
	serverName spec.ServerName,
	relayServers []spec.ServerName,
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
	serverName spec.ServerName,
) ([]spec.ServerName, error) {
	stmt := sqlutil.TxStmt(txn, s.selectRelayServersStmt)
	rows, err := stmt.QueryContext(ctx, serverName)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectRelayServers: rows.close() failed")

	var result []spec.ServerName
	for rows.Next() {
		var relayServer string
		if err = rows.Scan(&relayServer); err != nil {
			return nil, err
		}
		result = append(result, spec.ServerName(relayServer))
	}
	return result, rows.Err()
}

func (s *relayServersStatements) DeleteRelayServers(
	ctx context.Context,
	txn *sql.Tx,
	serverName spec.ServerName,
	relayServers []spec.ServerName,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteRelayServersStmt)
	_, err := stmt.ExecContext(ctx, serverName, pq.Array(relayServers))
	return err
}

func (s *relayServersStatements) DeleteAllRelayServers(
	ctx context.Context,
	txn *sql.Tx,
	serverName spec.ServerName,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteAllRelayServersStmt)
	if _, err := stmt.ExecContext(ctx, serverName); err != nil {
		return err
	}
	return nil
}
