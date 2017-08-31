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

package accounts

import (
	"database/sql"
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
	selectLocalpartForThreePIDStmt  *sql.Stmt
	selectThreePIDsForLocalpartStmt *sql.Stmt
	insertThreePIDStmt              *sql.Stmt
	deleteThreePIDStmt              *sql.Stmt
}

func (s *threepidStatements) prepare(db *sql.DB) (err error) {
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

func (s *threepidStatements) selectLocalpartForThreePID(txn *sql.Tx, threepid string, medium string) (localpart string, err error) {
	var stmt *sql.Stmt
	if txn != nil {
		stmt = txn.Stmt(s.selectLocalpartForThreePIDStmt)
	} else {
		stmt = s.selectLocalpartForThreePIDStmt
	}
	err = stmt.QueryRow(threepid, medium).Scan(&localpart)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return
}

func (s *threepidStatements) selectThreePIDsForLocalpart(localpart string) (threepids map[string]string, err error) {
	rows, err := s.selectThreePIDsForLocalpartStmt.Query(localpart)
	if err != nil {
		return
	}

	threepids = make(map[string]string)
	for rows.Next() {
		var threepid string
		var medium string
		if err = rows.Scan(&threepid, &medium); err != nil {
			return
		}
		threepids[threepid] = medium
	}

	return
}

func (s *threepidStatements) insertThreePID(txn *sql.Tx, threepid string, medium string, localpart string) (err error) {
	_, err = txn.Stmt(s.insertThreePIDStmt).Exec(threepid, medium, localpart)
	return
}

func (s *threepidStatements) deleteThreePID(threepid string, medium string) (err error) {
	_, err = s.deleteThreePIDStmt.Exec(threepid, medium)
	return
}
