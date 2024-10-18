// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/userapi/storage/tables"
	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
)

const threepidSchema = `
-- Stores data about third party identifiers
CREATE TABLE IF NOT EXISTS userapi_threepids (
	-- The third party identifier
	threepid TEXT NOT NULL,
	-- The 3PID medium
	medium TEXT NOT NULL DEFAULT 'email',
	-- The localpart of the Matrix user ID associated to this 3PID
	localpart TEXT NOT NULL,
	server_name TEXT NOT NULL,

	PRIMARY KEY(threepid, medium)
);

CREATE INDEX IF NOT EXISTS userapi_threepid_idx ON userapi_threepids(localpart, server_name);
`

const selectLocalpartForThreePIDSQL = "" +
	"SELECT localpart, server_name FROM userapi_threepids WHERE threepid = $1 AND medium = $2"

const selectThreePIDsForLocalpartSQL = "" +
	"SELECT threepid, medium FROM userapi_threepids WHERE localpart = $1 AND server_name = $2"

const insertThreePIDSQL = "" +
	"INSERT INTO userapi_threepids (threepid, medium, localpart, server_name) VALUES ($1, $2, $3, $4)"

const deleteThreePIDSQL = "" +
	"DELETE FROM userapi_threepids WHERE threepid = $1 AND medium = $2"

type threepidStatements struct {
	selectLocalpartForThreePIDStmt  *sql.Stmt
	selectThreePIDsForLocalpartStmt *sql.Stmt
	insertThreePIDStmt              *sql.Stmt
	deleteThreePIDStmt              *sql.Stmt
}

func NewPostgresThreePIDTable(db *sql.DB) (tables.ThreePIDTable, error) {
	s := &threepidStatements{}
	_, err := db.Exec(threepidSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.selectLocalpartForThreePIDStmt, selectLocalpartForThreePIDSQL},
		{&s.selectThreePIDsForLocalpartStmt, selectThreePIDsForLocalpartSQL},
		{&s.insertThreePIDStmt, insertThreePIDSQL},
		{&s.deleteThreePIDStmt, deleteThreePIDSQL},
	}.Prepare(db)
}

func (s *threepidStatements) SelectLocalpartForThreePID(
	ctx context.Context, txn *sql.Tx, threepid string, medium string,
) (localpart string, serverName spec.ServerName, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectLocalpartForThreePIDStmt)
	err = stmt.QueryRowContext(ctx, threepid, medium).Scan(&localpart, &serverName)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	return
}

func (s *threepidStatements) SelectThreePIDsForLocalpart(
	ctx context.Context,
	localpart string, serverName spec.ServerName,
) (threepids []authtypes.ThreePID, err error) {
	rows, err := s.selectThreePIDsForLocalpartStmt.QueryContext(ctx, localpart, serverName)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectThreePIDsForLocalpart: failed to close rows")

	threepids = []authtypes.ThreePID{}
	for rows.Next() {
		var threepid string
		var medium string
		if err = rows.Scan(&threepid, &medium); err != nil {
			return
		}
		threepids = append(threepids, authtypes.ThreePID{
			Address: threepid,
			Medium:  medium,
		})
	}
	err = rows.Err()
	return
}

func (s *threepidStatements) InsertThreePID(
	ctx context.Context, txn *sql.Tx, threepid, medium,
	localpart string, serverName spec.ServerName,
) (err error) {
	stmt := sqlutil.TxStmt(txn, s.insertThreePIDStmt)
	_, err = stmt.ExecContext(ctx, threepid, medium, localpart, serverName)
	return
}

func (s *threepidStatements) DeleteThreePID(
	ctx context.Context, txn *sql.Tx, threepid string, medium string) (err error) {
	stmt := sqlutil.TxStmt(txn, s.deleteThreePIDStmt)
	_, err = stmt.ExecContext(ctx, threepid, medium)
	return
}
