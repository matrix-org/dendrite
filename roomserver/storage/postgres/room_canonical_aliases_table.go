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

package postgres

import (
	"context"
	"database/sql"
)

const roomCanonicalAliasSchema = `
-- Stores room canonical alias and room IDs they refer to
CREATE TABLE IF NOT EXISTS roomserver_canonical_aliases (
    -- Canonical alias of the room
    canonical_alias TEXT NOT NULL PRIMARY KEY,
    -- Room ID the canonical_alias refers to
    room_id TEXT NOT NULL,
    -- User ID of the creator of this canonical_alias
    creator_id TEXT NOT NULL

    FOREIGN KEY (canonical_alias) REFERENCES roomserver_room_aliases(alias)
);

CREATE INDEX IF NOT EXISTS roomserver_room_id_idx ON roomserver_canonical_aliases(room_id);
`

const insertRoomCanonicalAliasSQL = "" +
	"INSERT INTO roomserver_canonical_aliases (canonical_alias, room_id, creator_id) VALUES ($1, $2, $3)"

const selectRoomIDFromCanonicalAliasSQL = "" +
	"SELECT room_id FROM roomserver_canonical_aliases WHERE canonical_alias = $1"

const selectCanonicalAliasFromRoomIDSQL = "" +
	"SELECT canonical_alias FROM roomserver_canonical_aliases WHERE room_id = $1"

const selectCreatorIDFromCanonicalAliasSQL = "" +
	"SELECT creator_id FROM roomserver_canonical_aliases WHERE canonical_alias = $1"

const deleteRoomCanonicalAliasSQL = "" +
	"DELETE FROM roomserver_canonical_aliases WHERE canonical_alias = $1"

type roomCanonicalAliasStatements struct {
	insertRoomCanonicalAliasStmt          *sql.Stmt
	selectRoomIDFromCanonicalAliasStmt    *sql.Stmt
	selectCanonicalAliasFromRoomIDStmt    *sql.Stmt
	selectCreaterIDFromCanonicalAliasStmt *sql.Stmt
	deleteRoomCanonicalAliasStmt          *sql.Stmt
}

func (s *roomCanonicalAliasStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(roomCanonicalAliasSchema)
	if err != nil {
		return
	}
	return statementList{
		{&s.insertRoomCanonicalAliasStmt, insertRoomCanonicalAliasSQL},
		{&s.selectRoomIDFromCanonicalAliasStmt, selectRoomIDFromCanonicalAliasSQL},
		{&s.selectCanonicalAliasFromRoomIDStmt, selectCanonicalAliasFromRoomIDSQL},
		{&s.selectCreaterIDFromCanonicalAliasStmt, selectCreatorIDFromCanonicalAliasSQL},
		{&s.deleteRoomCanonicalAliasStmt, deleteRoomCanonicalAliasSQL},
	}.prepare(db)
}

func (s *roomCanonicalAliasStatements) insertRoomCanonicalAlias(
	ctx context.Context, canonical_alias string, roomID string, creatorUserID string,
) (err error) {
	_, err = s.insertRoomCanonicalAliasStmt.ExecContext(ctx, canonical_alias, roomID, creatorUserID)
	return
}

func (s *roomCanonicalAliasStatements) selectRoomIDFromCanonicalAlias(
	ctx context.Context, canonical_alias string,
) (roomID string, err error) {
	err = s.selectRoomIDFromCanonicalAliasStmt.QueryRowContext(ctx, canonical_alias).Scan(&roomID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return
}

func (s *roomCanonicalAliasStatements) selectCanonicalAliasFromRoomID(
	ctx context.Context, roomID string,
) (canonical_alias string, err error) {
	err = s.selectCanonicalAliasFromRoomIDStmt.QueryRowContext(ctx, roomID).Scan(&canonical_alias)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return
}

func (s *roomCanonicalAliasStatements) selectCreatorIDFromCanonicalAlias(
	ctx context.Context, canonical_alias string,
) (creatorID string, err error) {
	err = s.selectCreaterIDFromCanonicalAliasStmt.QueryRowContext(ctx, canonical_alias).Scan(&creatorID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return
}

func (s *roomCanonicalAliasStatements) deleteRoomCanonicalAlias(
	ctx context.Context, canonical_alias string,
) (err error) {
	_, err = s.deleteRoomCanonicalAliasStmt.ExecContext(ctx, canonical_alias)
	return
}
