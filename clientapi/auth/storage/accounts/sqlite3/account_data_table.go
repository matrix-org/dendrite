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

	"github.com/matrix-org/gomatrixserverlib"
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
	ON CONFLICT (localpart, room_id, type) DO UPDATE SET content = EXCLUDED.content
`

const selectAccountDataSQL = "" +
	"SELECT room_id, type, content FROM account_data WHERE localpart = $1"

const selectAccountDataByTypeSQL = "" +
	"SELECT content FROM account_data WHERE localpart = $1 AND room_id = $2 AND type = $3"

type accountDataStatements struct {
	insertAccountDataStmt       *sql.Stmt
	selectAccountDataStmt       *sql.Stmt
	selectAccountDataByTypeStmt *sql.Stmt
}

func (s *accountDataStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(accountDataSchema)
	if err != nil {
		return
	}
	if s.insertAccountDataStmt, err = db.Prepare(insertAccountDataSQL); err != nil {
		return
	}
	if s.selectAccountDataStmt, err = db.Prepare(selectAccountDataSQL); err != nil {
		return
	}
	if s.selectAccountDataByTypeStmt, err = db.Prepare(selectAccountDataByTypeSQL); err != nil {
		return
	}
	return
}

func (s *accountDataStatements) insertAccountData(
	ctx context.Context, localpart, roomID, dataType, content string,
) (err error) {
	stmt := s.insertAccountDataStmt
	_, err = stmt.ExecContext(ctx, localpart, roomID, dataType, content)
	return
}

func (s *accountDataStatements) selectAccountData(
	ctx context.Context, localpart string,
) (
	global []gomatrixserverlib.ClientEvent,
	rooms map[string][]gomatrixserverlib.ClientEvent,
	err error,
) {
	rows, err := s.selectAccountDataStmt.QueryContext(ctx, localpart)
	if err != nil {
		return
	}

	global = []gomatrixserverlib.ClientEvent{}
	rooms = make(map[string][]gomatrixserverlib.ClientEvent)

	for rows.Next() {
		var roomID string
		var dataType string
		var content []byte

		if err = rows.Scan(&roomID, &dataType, &content); err != nil {
			return
		}

		ac := gomatrixserverlib.ClientEvent{
			Type:    dataType,
			Content: content,
		}

		if len(roomID) > 0 {
			rooms[roomID] = append(rooms[roomID], ac)
		} else {
			global = append(global, ac)
		}
	}

	return
}

func (s *accountDataStatements) selectAccountDataByType(
	ctx context.Context, localpart, roomID, dataType string,
) (data *gomatrixserverlib.ClientEvent, err error) {
	stmt := s.selectAccountDataByTypeStmt
	var content []byte

	if err = stmt.QueryRowContext(ctx, localpart, roomID, dataType).Scan(&content); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}

		return
	}

	data = &gomatrixserverlib.ClientEvent{
		Type:    dataType,
		Content: content,
	}

	return
}
