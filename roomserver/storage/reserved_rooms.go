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
	"context"
	"database/sql"
	"time"

	"github.com/matrix-org/dendrite/common"
)

const reservedRoomSchema = `
-- The events table holds metadata for each event, the actual JSON is stored
-- separately to keep the size of the rows small.
CREATE SEQUENCE IF NOT EXISTS roomserver_event_nid_seq;
CREATE TABLE IF NOT EXISTS roomserver_reserved_room_ids (
	--- The Room ID ---
	room_id TEXT PRIMARY KEY,
	reserved_since TIMESTAMP NOT NULL
);
`

const reserveRoomIDSQL = "" +
	"INSERT INTO roomserver_reserved_room_ids (room_id, reserved_since) select $1, $2"
	//"WHERE NOT EXISTS (SELECT 1 FROM roomserver_rooms WHERE room_id = $1)"

const selectReservedRoomIDSQL = "" +
	"SELECT reserved_since FROM roomserver_reserved_room_ids WHERE room_id = $1"

type reservedRoomStatements struct {
	reserveRoomIDStmt        *sql.Stmt
	selectReservedRoomIDStmt *sql.Stmt
}

func (s *reservedRoomStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(reservedRoomSchema)
	if err != nil {
		return
	}

	return statementList{
		{&s.reserveRoomIDStmt, reserveRoomIDSQL},
		{&s.selectReservedRoomIDStmt, selectReservedRoomIDSQL},
	}.prepare(db)
}

func (s *reservedRoomStatements) reserveRoomID(
	ctx context.Context,
	roomID string,
	reservedSince time.Time,
) (success bool, err error) {
	_, err = s.reserveRoomIDStmt.ExecContext(ctx, roomID, reservedSince)

	if common.IsUniqueConstraintViolationErr(err) {
		return false, nil
	}

	return err == nil, err
}

func (s *reservedRoomStatements) selectReservedRoomID(
	ctx context.Context, roomID string,
) (time.Time, error) {
	var reservedSince time.Time
	err := s.selectReservedRoomIDStmt.QueryRowContext(ctx, roomID).Scan(&reservedSince)
	return reservedSince, err
}
