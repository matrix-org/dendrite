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

package storage

import (
	"database/sql"

	"github.com/lib/pq"
)

const roomAliasesSchema = `
-- Stores room aliases and room IDs they refer to
CREATE TABLE IF NOT EXISTS roomserver_room_aliases (
    -- Alias of the room
    alias TEXT NOT NULL PRIMARY KEY,
    -- Room ID the alias refers to
    room_id TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS roomserver_room_id_idx ON roomserver_room_aliases(room_id);
`

const insertRoomAliasSQL = "" +
	"INSERT INTO roomserver_room_aliases (alias, room_id) VALUES ($1, $2)"

const selectRoomIDFromAliasSQL = "" +
	"SELECT room_id FROM roomserver_room_aliases WHERE alias = $1"

const selectAliasesFromRoomIDSQL = "" +
	"SELECT alias FROM roomserver_room_aliases WHERE room_id = $1"

const selectAliasesFromRoomIDsSQL = "" +
	"SELECT alias, room_id FROM roomserver_room_aliases WHERE room_id = ANY($1)"

const deleteRoomAliasSQL = "" +
	"DELETE FROM roomserver_room_aliases WHERE alias = $1"

type roomAliasesStatements struct {
	insertRoomAliasStmt          *sql.Stmt
	selectRoomIDFromAliasStmt    *sql.Stmt
	selectAliasesFromRoomIDStmt  *sql.Stmt
	selectAliasesFromRoomIDsStmt *sql.Stmt
	deleteRoomAliasStmt          *sql.Stmt
}

func (s *roomAliasesStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(roomAliasesSchema)
	if err != nil {
		return
	}
	return statementList{
		{&s.insertRoomAliasStmt, insertRoomAliasSQL},
		{&s.selectRoomIDFromAliasStmt, selectRoomIDFromAliasSQL},
		{&s.selectAliasesFromRoomIDStmt, selectAliasesFromRoomIDSQL},
		{&s.selectAliasesFromRoomIDsStmt, selectAliasesFromRoomIDsSQL},
		{&s.deleteRoomAliasStmt, deleteRoomAliasSQL},
	}.prepare(db)
}

func (s *roomAliasesStatements) insertRoomAlias(alias string, roomID string) (err error) {
	_, err = s.insertRoomAliasStmt.Exec(alias, roomID)
	return
}

func (s *roomAliasesStatements) selectRoomIDFromAlias(alias string) (roomID string, err error) {
	err = s.selectRoomIDFromAliasStmt.QueryRow(alias).Scan(&roomID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return
}

func (s *roomAliasesStatements) selectAliasesFromRoomID(roomID string) (aliases []string, err error) {
	aliases = []string{}
	rows, err := s.selectAliasesFromRoomIDStmt.Query(roomID)
	if err != nil {
		return
	}

	for rows.Next() {
		var alias string
		if err = rows.Scan(&alias); err != nil {
			return
		}

		aliases = append(aliases, alias)
	}

	return
}

func (s *roomAliasesStatements) selectAliasesFromRoomIDs(roomIDs []string) (aliases map[string][]string, err error) {
	aliases = make(map[string][]string)
	rows, err := s.selectAliasesFromRoomIDsStmt.Query(pq.StringArray(roomIDs))
	if err != nil {
		return
	}

	for rows.Next() {
		var alias, roomID string
		if err = rows.Scan(&alias, &roomID); err != nil {
			return
		}

		if len(aliases[roomID]) > 0 {
			aliases[roomID] = append(aliases[roomID], alias)
		} else {
			aliases[roomID] = []string{alias}
		}
	}

	return
}

func (s *roomAliasesStatements) deleteRoomAlias(alias string) (err error) {
	_, err = s.deleteRoomAliasStmt.Exec(alias)
	return
}
