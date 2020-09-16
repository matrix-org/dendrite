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

package sqlite3

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
)

const threepidSchema = `
-- Stores data about third party identifiers
CREATE TABLE IF NOT EXISTS account_threepid (
	-- The third party identifier
	threepid TEXT NOT NULL,
	-- The 3PID medium
	medium TEXT NOT NULL DEFAULT 'email',
	-- The localpart of the Matrix user ID associated to this 3PID
	localpart TEXT NOT NULL,

	PRIMARY KEY(threepid, medium)
);

CREATE INDEX IF NOT EXISTS account_threepid_localpart ON account_threepid(localpart);
`

const selectLocalpartForThreePIDSQL = "" +
	"SELECT localpart FROM account_threepid WHERE threepid = $1 AND medium = $2"

const selectThreePIDsForLocalpartSQL = "" +
	"SELECT threepid, medium FROM account_threepid WHERE localpart = $1"

const insertThreePIDSQL = "" +
	"INSERT INTO account_threepid (threepid, medium, localpart) VALUES ($1, $2, $3)"

const deleteThreePIDSQL = "" +
	"DELETE FROM account_threepid WHERE threepid = $1 AND medium = $2"

type threepidStatements struct {
	db                              *sql.DB
	selectLocalpartForThreePIDStmt  *sql.Stmt
	selectThreePIDsForLocalpartStmt *sql.Stmt
	insertThreePIDStmt              *sql.Stmt
	deleteThreePIDStmt              *sql.Stmt
}

func (s *threepidStatements) prepare(db *sql.DB) (err error) {
	s.db = db
	_, err = db.Exec(threepidSchema)
	if err != nil {
		return
	}
	if s.selectLocalpartForThreePIDStmt, err = db.Prepare(selectLocalpartForThreePIDSQL); err != nil {
		return
	}
	if s.selectThreePIDsForLocalpartStmt, err = db.Prepare(selectThreePIDsForLocalpartSQL); err != nil {
		return
	}
	if s.insertThreePIDStmt, err = db.Prepare(insertThreePIDSQL); err != nil {
		return
	}
	if s.deleteThreePIDStmt, err = db.Prepare(deleteThreePIDSQL); err != nil {
		return
	}

	return
}

func (s *threepidStatements) selectLocalpartForThreePID(
	ctx context.Context, txn *sql.Tx, threepid string, medium string,
) (localpart string, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectLocalpartForThreePIDStmt)
	err = stmt.QueryRowContext(ctx, threepid, medium).Scan(&localpart)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return
}

func (s *threepidStatements) selectThreePIDsForLocalpart(
	ctx context.Context, localpart string,
) (threepids []authtypes.ThreePID, err error) {
	rows, err := s.selectThreePIDsForLocalpartStmt.QueryContext(ctx, localpart)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectThreePIDsForLocalpart: rows.close() failed")

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
	return threepids, rows.Err()
}

func (s *threepidStatements) insertThreePID(
	ctx context.Context, txn *sql.Tx, threepid, medium, localpart string,
) (err error) {
	stmt := sqlutil.TxStmt(txn, s.insertThreePIDStmt)
	_, err = stmt.ExecContext(ctx, threepid, medium, localpart)
	return err
}

func (s *threepidStatements) deleteThreePID(
	ctx context.Context, txn *sql.Tx, threepid string, medium string) (err error) {
	stmt := sqlutil.TxStmt(txn, s.deleteThreePIDStmt)
	_, err = stmt.ExecContext(ctx, threepid, medium)
	return err
}
