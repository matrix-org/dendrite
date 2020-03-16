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
	"encoding/json"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

const roomsSchema = `
  CREATE TABLE IF NOT EXISTS roomserver_rooms (
    room_nid INTEGER PRIMARY KEY AUTOINCREMENT,
    room_id TEXT NOT NULL UNIQUE,
    latest_event_nids TEXT NOT NULL DEFAULT '[]',
    last_event_sent_nid INTEGER NOT NULL DEFAULT 0,
    state_snapshot_nid INTEGER NOT NULL DEFAULT 0,
    room_version TEXT NOT NULL
  );
`

// Same as insertEventTypeNIDSQL
const insertRoomNIDSQL = `
	INSERT INTO roomserver_rooms (room_id, room_version) VALUES ($1, $2)
	  ON CONFLICT DO NOTHING;
`

const selectRoomNIDSQL = "" +
	"SELECT room_nid FROM roomserver_rooms WHERE room_id = $1"

const selectLatestEventNIDsSQL = "" +
	"SELECT latest_event_nids, state_snapshot_nid FROM roomserver_rooms WHERE room_nid = $1"

const selectLatestEventNIDsForUpdateSQL = "" +
	"SELECT latest_event_nids, last_event_sent_nid, state_snapshot_nid FROM roomserver_rooms WHERE room_nid = $1"

const updateLatestEventNIDsSQL = "" +
	"UPDATE roomserver_rooms SET latest_event_nids = $1, last_event_sent_nid = $2, state_snapshot_nid = $3 WHERE room_nid = $4"

const selectRoomVersionForRoomNIDSQL = "" +
	"SELECT room_version FROM roomserver_rooms WHERE room_nid = $1"

type roomStatements struct {
	insertRoomNIDStmt                  *sql.Stmt
	selectRoomNIDStmt                  *sql.Stmt
	selectLatestEventNIDsStmt          *sql.Stmt
	selectLatestEventNIDsForUpdateStmt *sql.Stmt
	updateLatestEventNIDsStmt          *sql.Stmt
	selectRoomVersionForRoomNIDStmt    *sql.Stmt
}

func (s *roomStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(roomsSchema)
	if err != nil {
		return
	}
	return statementList{
		{&s.insertRoomNIDStmt, insertRoomNIDSQL},
		{&s.selectRoomNIDStmt, selectRoomNIDSQL},
		{&s.selectLatestEventNIDsStmt, selectLatestEventNIDsSQL},
		{&s.selectLatestEventNIDsForUpdateStmt, selectLatestEventNIDsForUpdateSQL},
		{&s.updateLatestEventNIDsStmt, updateLatestEventNIDsSQL},
		{&s.selectRoomVersionForRoomNIDStmt, selectRoomVersionForRoomNIDSQL},
	}.prepare(db)
}

func (s *roomStatements) insertRoomNID(
	ctx context.Context, txn *sql.Tx,
	roomID string, roomVersion gomatrixserverlib.RoomVersion,
) (types.RoomNID, error) {
	var err error
	insertStmt := common.TxStmt(txn, s.insertRoomNIDStmt)
	if _, err = insertStmt.ExecContext(ctx, roomID, roomVersion); err == nil {
		return s.selectRoomNID(ctx, txn, roomID)
	} else {
		return types.RoomNID(0), err
	}
}

func (s *roomStatements) selectRoomNID(
	ctx context.Context, txn *sql.Tx, roomID string,
) (types.RoomNID, error) {
	var roomNID int64
	stmt := common.TxStmt(txn, s.selectRoomNIDStmt)
	err := stmt.QueryRowContext(ctx, roomID).Scan(&roomNID)
	return types.RoomNID(roomNID), err
}

func (s *roomStatements) selectLatestEventNIDs(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) ([]types.EventNID, types.StateSnapshotNID, error) {
	var eventNIDs []types.EventNID
	var nidsJSON string
	var stateSnapshotNID int64
	stmt := common.TxStmt(txn, s.selectLatestEventNIDsStmt)
	err := stmt.QueryRowContext(ctx, int64(roomNID)).Scan(&nidsJSON, &stateSnapshotNID)
	if err != nil {
		return nil, 0, err
	}
	if err := json.Unmarshal([]byte(nidsJSON), &eventNIDs); err != nil {
		return nil, 0, err
	}
	return eventNIDs, types.StateSnapshotNID(stateSnapshotNID), nil
}

func (s *roomStatements) selectLatestEventsNIDsForUpdate(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) ([]types.EventNID, types.EventNID, types.StateSnapshotNID, error) {
	var eventNIDs []types.EventNID
	var nidsJSON string
	var lastEventSentNID int64
	var stateSnapshotNID int64
	stmt := common.TxStmt(txn, s.selectLatestEventNIDsForUpdateStmt)
	err := stmt.QueryRowContext(ctx, int64(roomNID)).Scan(&nidsJSON, &lastEventSentNID, &stateSnapshotNID)
	if err != nil {
		return nil, 0, 0, err
	}
	if err := json.Unmarshal([]byte(nidsJSON), &eventNIDs); err != nil {
		return nil, 0, 0, err
	}
	return eventNIDs, types.EventNID(lastEventSentNID), types.StateSnapshotNID(stateSnapshotNID), nil
}

func (s *roomStatements) updateLatestEventNIDs(
	ctx context.Context,
	txn *sql.Tx,
	roomNID types.RoomNID,
	eventNIDs []types.EventNID,
	lastEventSentNID types.EventNID,
	stateSnapshotNID types.StateSnapshotNID,
) error {
	stmt := common.TxStmt(txn, s.updateLatestEventNIDsStmt)
	_, err := stmt.ExecContext(
		ctx,
		eventNIDsAsArray(eventNIDs),
		int64(lastEventSentNID),
		int64(stateSnapshotNID),
		roomNID,
	)
	return err
}

func (s *roomStatements) selectRoomVersionForRoomNID(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) (gomatrixserverlib.RoomVersion, error) {
	var roomVersion gomatrixserverlib.RoomVersion
	stmt := common.TxStmt(txn, s.selectRoomVersionForRoomNIDStmt)
	err := stmt.QueryRowContext(ctx, roomNID).Scan(&roomVersion)
	return roomVersion, err
}
