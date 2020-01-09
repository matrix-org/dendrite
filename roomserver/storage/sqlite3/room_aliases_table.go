// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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
)

const roomAliasesSchema = `
  CREATE TABLE IF NOT EXISTS roomserver_room_aliases (
    alias TEXT NOT NULL PRIMARY KEY,
    room_id TEXT NOT NULL,
    creator_id TEXT NOT NULL
  );

  CREATE INDEX IF NOT EXISTS roomserver_room_id_idx ON roomserver_room_aliases(room_id);
`

const insertRoomAliasSQL = `
	INSERT INTO roomserver_room_aliases (alias, room_id, creator_id) VALUES ($1, $2, $3)
`

const selectRoomIDFromAliasSQL = `
	SELECT room_id FROM roomserver_room_aliases WHERE alias = $1
`

const selectAliasesFromRoomIDSQL = `
	SELECT alias FROM roomserver_room_aliases WHERE room_id = $1
`

const selectCreatorIDFromAliasSQL = `
	SELECT creator_id FROM roomserver_room_aliases WHERE alias = $1
`

const deleteRoomAliasSQL = `
	DELETE FROM roomserver_room_aliases WHERE alias = $1
`

type roomAliasesStatements struct {
	insertRoomAliasStmt          *sql.Stmt
	selectRoomIDFromAliasStmt    *sql.Stmt
	selectAliasesFromRoomIDStmt  *sql.Stmt
	selectCreatorIDFromAliasStmt *sql.Stmt
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
		{&s.selectCreatorIDFromAliasStmt, selectCreatorIDFromAliasSQL},
		{&s.deleteRoomAliasStmt, deleteRoomAliasSQL},
	}.prepare(db)
}

func (s *roomAliasesStatements) insertRoomAlias(
	ctx context.Context, alias string, roomID string, creatorUserID string,
) (err error) {
	_, err = s.insertRoomAliasStmt.ExecContext(ctx, alias, roomID, creatorUserID)
	return
}

func (s *roomAliasesStatements) selectRoomIDFromAlias(
	ctx context.Context, alias string,
) (roomID string, err error) {
	err = s.selectRoomIDFromAliasStmt.QueryRowContext(ctx, alias).Scan(&roomID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return
}

func (s *roomAliasesStatements) selectAliasesFromRoomID(
	ctx context.Context, roomID string,
) (aliases []string, err error) {
	aliases = []string{}
	rows, err := s.selectAliasesFromRoomIDStmt.QueryContext(ctx, roomID)
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

func (s *roomAliasesStatements) selectCreatorIDFromAlias(
	ctx context.Context, alias string,
) (creatorID string, err error) {
	err = s.selectCreatorIDFromAliasStmt.QueryRowContext(ctx, alias).Scan(&creatorID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return
}

func (s *roomAliasesStatements) deleteRoomAlias(
	ctx context.Context, alias string,
) (err error) {
	_, err = s.deleteRoomAliasStmt.ExecContext(ctx, alias)
	return
}
