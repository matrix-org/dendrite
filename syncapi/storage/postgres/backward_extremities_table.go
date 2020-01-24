// Copyright 2018 New Vector Ltd
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

const backwardExtremitiesSchema = `
-- Stores output room events received from the roomserver.
CREATE TABLE IF NOT EXISTS syncapi_backward_extremities (
	-- The 'room_id' key for the event.
	room_id TEXT NOT NULL,
	-- The event ID for the event.
	event_id TEXT NOT NULL,

	PRIMARY KEY(room_id, event_id)
);
`

const insertBackwardExtremitySQL = "" +
	"INSERT INTO syncapi_backward_extremities (room_id, event_id)" +
	" VALUES ($1, $2)" +
	" ON CONFLICT DO NOTHING"

const selectBackwardExtremitiesForRoomSQL = "" +
	"SELECT event_id FROM syncapi_backward_extremities WHERE room_id = $1"

const isBackwardExtremitySQL = "" +
	"SELECT EXISTS (" +
	" SELECT TRUE FROM syncapi_backward_extremities" +
	" WHERE room_id = $1 AND event_id = $2" +
	")"

const deleteBackwardExtremitySQL = "" +
	"DELETE FROM syncapi_backward_extremities WHERE room_id = $1 AND event_id = $2"

type backwardExtremitiesStatements struct {
	insertBackwardExtremityStmt          *sql.Stmt
	selectBackwardExtremitiesForRoomStmt *sql.Stmt
	isBackwardExtremityStmt              *sql.Stmt
	deleteBackwardExtremityStmt          *sql.Stmt
}

func (s *backwardExtremitiesStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(backwardExtremitiesSchema)
	if err != nil {
		return
	}
	if s.insertBackwardExtremityStmt, err = db.Prepare(insertBackwardExtremitySQL); err != nil {
		return
	}
	if s.selectBackwardExtremitiesForRoomStmt, err = db.Prepare(selectBackwardExtremitiesForRoomSQL); err != nil {
		return
	}
	if s.isBackwardExtremityStmt, err = db.Prepare(isBackwardExtremitySQL); err != nil {
		return
	}
	if s.deleteBackwardExtremityStmt, err = db.Prepare(deleteBackwardExtremitySQL); err != nil {
		return
	}
	return
}

func (s *backwardExtremitiesStatements) insertsBackwardExtremity(
	ctx context.Context, roomID, eventID string,
) (err error) {
	_, err = s.insertBackwardExtremityStmt.ExecContext(ctx, roomID, eventID)
	return
}

func (s *backwardExtremitiesStatements) selectBackwardExtremitiesForRoom(
	ctx context.Context, roomID string,
) (eventIDs []string, err error) {
	eventIDs = make([]string, 0)

	rows, err := s.selectBackwardExtremitiesForRoomStmt.QueryContext(ctx, roomID)
	if err != nil {
		return
	}

	for rows.Next() {
		var eID string
		if err = rows.Scan(&eID); err != nil {
			return
		}

		eventIDs = append(eventIDs, eID)
	}

	return
}

func (s *backwardExtremitiesStatements) isBackwardExtremity(
	ctx context.Context, roomID, eventID string,
) (isBE bool, err error) {
	err = s.isBackwardExtremityStmt.QueryRowContext(ctx, roomID, eventID).Scan(&isBE)
	return
}

func (s *backwardExtremitiesStatements) deleteBackwardExtremity(
	ctx context.Context, roomID, eventID string,
) (err error) {
	_, err = s.insertBackwardExtremityStmt.ExecContext(ctx, roomID, eventID)
	return
}
