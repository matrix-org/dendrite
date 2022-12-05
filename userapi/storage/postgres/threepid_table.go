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

package postgres

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
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
) (localpart string, serverName gomatrixserverlib.ServerName, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectLocalpartForThreePIDStmt)
	err = stmt.QueryRowContext(ctx, threepid, medium).Scan(&localpart, &serverName)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	return
}

func (s *threepidStatements) SelectThreePIDsForLocalpart(
	ctx context.Context,
	localpart string, serverName gomatrixserverlib.ServerName,
) (threepids []authtypes.ThreePID, err error) {
	rows, err := s.selectThreePIDsForLocalpartStmt.QueryContext(ctx, localpart, serverName)
	if err != nil {
		return
	}

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

	return
}

func (s *threepidStatements) InsertThreePID(
	ctx context.Context, txn *sql.Tx, threepid, medium,
	localpart string, serverName gomatrixserverlib.ServerName,
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
