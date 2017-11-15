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
)

const reservedRoomSchema = `
-- The events table holds metadata for each event, the actual JSON is stored
-- separately to keep the size of the rows small.
CREATE SEQUENCE IF NOT EXISTS roomserver_event_nid_seq;
CREATE TABLE IF NOT EXISTS roomserver_reserved_rooms (
	--- The Room ID ---
	room_id TEXT PRIMARY KEY,
	reserved_since TIMESTAMP NOT NULL
);
`

const insertReservedRoomSQL = "" +
	"INSERT INTO roomserver_reserved_rooms (room_id, reserved_since)" +
	" VALUES ($1, $2)"

const selectReservedRoomSQL = "" +
	"SELECT reserved_since FROM roomserver_reserved_rooms WHERE room_id = $1"

type reservedRoomStatements struct {
	insertReservedRoomStmt *sql.Stmt
	selectReservedRoomStmt *sql.Stmt
}

func (s *reservedRoomStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(reservedRoomSchema)
	if err != nil {
		return
	}

	return statementList{
		{&s.insertReservedRoomStmt, insertReservedRoomSQL},
		{&s.selectReservedRoomStmt, selectReservedRoomSQL},
	}.prepare(db)
}

func (s *reservedRoomStatements) insertReservedRoom(
	ctx context.Context,
	roomID string,
	reservedSince time.Time,
) error {
	_, err := s.insertReservedRoomStmt.ExecContext(ctx, roomID, reservedSince)
	return err
}

func (s *reservedRoomStatements) selectReservedRoom(
	ctx context.Context, roomID string,
) (time.Time, error) {
	var reservedSince time.Time
	err := s.selectReservedRoomStmt.QueryRowContext(ctx, roomID).Scan(&reservedSince)
	return reservedSince, err
}
