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
	"encoding/json"

	"github.com/matrix-org/dendrite/internal/sqlutil"
)

const accountDataSchema = `
-- Stores data about accounts data.
CREATE TABLE IF NOT EXISTS account_data (
    -- The Matrix user ID localpart for this account
    localpart TEXT NOT NULL,
    -- The room ID for this data (empty string if not specific to a room)
    room_id TEXT,
    -- The account data type
    type TEXT NOT NULL,
    -- The account data content
    content TEXT NOT NULL,

    PRIMARY KEY(localpart, room_id, type)
);
`

const insertAccountDataSQL = `
	INSERT INTO account_data(localpart, room_id, type, content) VALUES($1, $2, $3, $4)
	ON CONFLICT (localpart, room_id, type) DO UPDATE SET content = $4
`

const selectAccountDataSQL = "" +
	"SELECT room_id, type, content FROM account_data WHERE localpart = $1"

const selectAccountDataByTypeSQL = "" +
	"SELECT content FROM account_data WHERE localpart = $1 AND room_id = $2 AND type = $3"

type accountDataStatements struct {
	db                          *sql.DB
	insertAccountDataStmt       *sql.Stmt
	selectAccountDataStmt       *sql.Stmt
	selectAccountDataByTypeStmt *sql.Stmt
}

func (s *accountDataStatements) prepare(db *sql.DB) (err error) {
	s.db = db
	_, err = db.Exec(accountDataSchema)
	if err != nil {
		return
	}
	return sqlutil.StatementList{
		{&s.insertAccountDataStmt, insertAccountDataSQL},
		{&s.selectAccountDataStmt, selectAccountDataSQL},
		{&s.selectAccountDataByTypeStmt, selectAccountDataByTypeSQL},
	}.Prepare(db)
}

func (s *accountDataStatements) insertAccountData(
	ctx context.Context, txn *sql.Tx, localpart, roomID, dataType string, content json.RawMessage,
) error {
	_, err := sqlutil.TxStmt(txn, s.insertAccountDataStmt).ExecContext(ctx, localpart, roomID, dataType, content)
	return err
}

func (s *accountDataStatements) selectAccountData(
	ctx context.Context, localpart string,
) (
	/* global */ map[string]json.RawMessage,
	/* rooms */ map[string]map[string]json.RawMessage,
	error,
) {
	rows, err := s.selectAccountDataStmt.QueryContext(ctx, localpart)
	if err != nil {
		return nil, nil, err
	}

	global := map[string]json.RawMessage{}
	rooms := map[string]map[string]json.RawMessage{}

	for rows.Next() {
		var roomID string
		var dataType string
		var content []byte

		if err = rows.Scan(&roomID, &dataType, &content); err != nil {
			return nil, nil, err
		}

		if roomID != "" {
			if _, ok := rooms[roomID]; !ok {
				rooms[roomID] = map[string]json.RawMessage{}
			}
			rooms[roomID][dataType] = content
		} else {
			global[dataType] = content
		}
	}

	return global, rooms, nil
}

func (s *accountDataStatements) selectAccountDataByType(
	ctx context.Context, localpart, roomID, dataType string,
) (data json.RawMessage, err error) {
	var bytes []byte
	stmt := s.selectAccountDataByTypeStmt
	if err = stmt.QueryRowContext(ctx, localpart, roomID, dataType).Scan(&bytes); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return
	}
	data = json.RawMessage(bytes)
	return
}
