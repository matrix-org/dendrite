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
	"github.com/matrix-org/dendrite/roomserver/types"
)

const roomAliasesSchema = `
-- Stores room aliases and room IDs they refer to
CREATE TABLE IF NOT EXISTS roomserver_room_aliases (
    -- Alias of the room
    alias TEXT NOT NULL PRIMARY KEY,
    -- Room numeric ID the alias refers to
    room_nid TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS roomserver_room_nid_idx ON roomserver_room_aliases(room_nid);
`

const insertRoomAliasSQL = "" +
	"INSERT INTO roomserver_room_aliases (alias, room_nid) VALUES ($1, $2)"

const selectRoomNIDFromAliasSQL = "" +
	"SELECT room_nid FROM roomserver_room_aliases WHERE alias = $1"

const selectAliasesFromRoomNIDSQL = "" +
	"SELECT alias FROM roomserver_room_aliases WHERE room_nid = $1"

const selectAliasesFromRoomNIDsSQL = "" +
	"SELECT alias, room_nid FROM roomserver_room_aliases WHERE room_nid = ANY($1)"

const deleteRoomAliasSQL = "" +
	"DELETE FROM roomserver_room_aliases WHERE alias = $1"

type roomAliasesStatements struct {
	insertRoomAliasStmt           *sql.Stmt
	selectRoomNIDFromAliasStmt    *sql.Stmt
	selectAliasesFromRoomNIDStmt  *sql.Stmt
	selectAliasesFromRoomNIDsStmt *sql.Stmt
	deleteRoomAliasStmt           *sql.Stmt
}

func (s *roomAliasesStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(roomAliasesSchema)
	if err != nil {
		return
	}
	return statementList{
		{&s.insertRoomAliasStmt, insertRoomAliasSQL},
		{&s.selectRoomNIDFromAliasStmt, selectRoomNIDFromAliasSQL},
		{&s.selectAliasesFromRoomNIDStmt, selectAliasesFromRoomNIDSQL},
		{&s.selectAliasesFromRoomNIDsStmt, selectAliasesFromRoomNIDsSQL},
		{&s.deleteRoomAliasStmt, deleteRoomAliasSQL},
	}.prepare(db)
}

func (s *roomAliasesStatements) insertRoomAlias(alias string, roomNID types.RoomNID) (err error) {
	_, err = s.insertRoomAliasStmt.Exec(alias, roomNID)
	return
}

func (s *roomAliasesStatements) selectRoomNIDFromAlias(alias string) (roomNID types.RoomNID, err error) {
	err = s.selectRoomNIDFromAliasStmt.QueryRow(alias).Scan(&roomNID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return
}

func (s *roomAliasesStatements) selectAliasesFromRoomNID(roomNID types.RoomNID) (aliases []string, err error) {
	aliases = []string{}
	rows, err := s.selectAliasesFromRoomNIDStmt.Query(roomNID)
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

func (s *roomAliasesStatements) selectAliasesFromRoomNIDs(roomNIDs []types.RoomNID) (aliases map[types.RoomNID][]string, err error) {
	aliases = make(map[types.RoomNID][]string)
	// Convert the array of numeric IDs into one that can be used in the query
	nIDs := []int64{}
	for _, roomNID := range roomNIDs {
		nIDs = append(nIDs, int64(roomNID))
	}
	rows, err := s.selectAliasesFromRoomNIDsStmt.Query(pq.Int64Array(nIDs))

	if err != nil {
		return
	}

	for rows.Next() {
		var alias string
		var roomNID types.RoomNID

		if err = rows.Scan(&alias, &roomNID); err != nil {
			return
		}

		if len(aliases[roomNID]) > 0 {
			aliases[roomNID] = append(aliases[roomNID], alias)
		} else {
			aliases[roomNID] = []string{alias}
		}
	}

	return
}

func (s *roomAliasesStatements) deleteRoomAlias(alias string) (err error) {
	_, err = s.deleteRoomAliasStmt.Exec(alias)
	return
}
