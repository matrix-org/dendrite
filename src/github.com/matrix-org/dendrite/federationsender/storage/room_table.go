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
)

const roomSchema = `
CREATE TABLE IF NOT EXISTS rooms (
    -- The string ID of the room
    room_id TEXT PRIMARY KEY,
    -- The most recent event state by the room server.
    -- We can use this to tell if our view of the room state has become
    -- desynchronised.
    last_event_id TEXT NOT NULL
);`

const insertRoomSQL = "" +
	"INSERT INTO rooms (room_id, last_event_id) VALUES ($1, '')" +
	" ON CONFLICT DO NOTHING"

const selectRoomForUpdateSQL = "" +
	"SELECT last_event_id FROM rooms WHERE room_id = $1 FOR UPDATE"

const updateRoomSQL = "" +
	"UPDATE rooms SET last_event_id = $2 WHERE room_id = $1"

type roomStatements struct {
	insertRoomStmt          *sql.Stmt
	selectRoomForUpdateStmt *sql.Stmt
	updateRoomStmt          *sql.Stmt
}

func (s *roomStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(roomSchema)
	if err != nil {
		return
	}

	if s.insertRoomStmt, err = db.Prepare(insertRoomSQL); err != nil {
		return
	}
	if s.selectRoomForUpdateStmt, err = db.Prepare(selectRoomForUpdateSQL); err != nil {
		return
	}
	if s.updateRoomStmt, err = db.Prepare(updateRoomSQL); err != nil {
		return
	}
	return
}

// insertRoom inserts the room if it didn't already exist.
// If the room didn't exist then last_event_id is set to the empty string.
func (s *roomStatements) insertRoom(txn *sql.Tx, roomID string) error {
	_, err := txn.Stmt(s.insertRoomStmt).Exec(roomID)
	return err
}

// selectRoomForUpdate locks the row for the room and returns the last_event_id.
// The row must already exist in the table. Callers can ensure that the row
// exists by calling insertRoom first.
func (s *roomStatements) selectRoomForUpdate(txn *sql.Tx, roomID string) (string, error) {
	var lastEventID string
	err := txn.Stmt(s.selectRoomForUpdateStmt).QueryRow(roomID).Scan(&lastEventID)
	if err != nil {
		return "", err
	}
	return lastEventID, nil
}

// updateRoom updates the last_event_id for the room. selectRoomForUpdate should
// have already been called earlier within the transaction.
func (s *roomStatements) updateRoom(txn *sql.Tx, roomID, lastEventID string) error {
	_, err := txn.Stmt(s.updateRoomStmt).Exec(roomID, lastEventID)
	return err
}
