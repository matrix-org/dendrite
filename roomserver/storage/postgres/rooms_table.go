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
	"fmt"
	"runtime/debug"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

const roomsSchema = `
CREATE SEQUENCE IF NOT EXISTS roomserver_room_nid_seq;
CREATE TABLE IF NOT EXISTS roomserver_rooms (
    -- Local numeric ID for the room.
    room_nid BIGINT PRIMARY KEY DEFAULT nextval('roomserver_room_nid_seq'),
    -- Textual ID for the room.
    room_id TEXT NOT NULL CONSTRAINT roomserver_room_id_unique UNIQUE,
    -- The most recent events in the room that aren't referenced by another event.
    -- This list may empty if the server hasn't joined the room yet.
    -- (The server will be in that state while it stores the events for the initial state of the room)
    latest_event_nids BIGINT[] NOT NULL DEFAULT '{}'::BIGINT[],
    -- The last event written to the output log for this room.
    last_event_sent_nid BIGINT NOT NULL DEFAULT 0,
    -- The state of the room after the current set of latest events.
    -- This will be 0 if there are no latest events in the room.
    state_snapshot_nid BIGINT NOT NULL DEFAULT 0,
    -- The version of the room, which will assist in determining the state resolution
    -- algorithm, event ID format, etc.
    room_version TEXT NOT NULL
);
`

// Same as insertEventTypeNIDSQL
const insertRoomNIDSQL = "" +
	"INSERT INTO roomserver_rooms (room_id, room_version) VALUES ($1, $2)" +
	" ON CONFLICT ON CONSTRAINT roomserver_room_id_unique" +
	" DO NOTHING RETURNING (room_nid)"

const selectRoomNIDSQL = "" +
	"SELECT room_nid FROM roomserver_rooms WHERE room_id = $1"

const selectLatestEventNIDsSQL = "" +
	"SELECT latest_event_nids, state_snapshot_nid FROM roomserver_rooms WHERE room_nid = $1"

const selectLatestEventNIDsForUpdateSQL = "" +
	"SELECT latest_event_nids, last_event_sent_nid, state_snapshot_nid FROM roomserver_rooms WHERE room_nid = $1 FOR UPDATE"

const updateLatestEventNIDsSQL = "" +
	"UPDATE roomserver_rooms SET latest_event_nids = $2, last_event_sent_nid = $3, state_snapshot_nid = $4 WHERE room_nid = $1"

const selectRoomVersionForRoomIDSQL = "" +
	"SELECT room_version FROM roomserver_rooms WHERE room_id = $1"

const selectRoomVersionForRoomNIDSQL = "" +
	"SELECT room_version FROM roomserver_rooms WHERE room_nid = $1"

type roomStatements struct {
	insertRoomNIDStmt                  *sql.Stmt
	selectRoomNIDStmt                  *sql.Stmt
	selectLatestEventNIDsStmt          *sql.Stmt
	selectLatestEventNIDsForUpdateStmt *sql.Stmt
	updateLatestEventNIDsStmt          *sql.Stmt
	selectRoomVersionForRoomIDStmt     *sql.Stmt
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
		{&s.selectRoomVersionForRoomIDStmt, selectRoomVersionForRoomIDSQL},
		{&s.selectRoomVersionForRoomNIDStmt, selectRoomVersionForRoomNIDSQL},
	}.prepare(db)
}

func (s *roomStatements) insertRoomNID(
	ctx context.Context, txn *sql.Tx,
	roomID string, roomVersion gomatrixserverlib.RoomVersion,
) (types.RoomNID, error) {
	var roomNID int64
	stmt := common.TxStmt(txn, s.insertRoomNIDStmt)
	err := stmt.QueryRowContext(ctx, roomID, roomVersion).Scan(&roomNID)
	return types.RoomNID(roomNID), err
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
	ctx context.Context, roomNID types.RoomNID,
) ([]types.EventNID, types.StateSnapshotNID, error) {
	var nids pq.Int64Array
	var stateSnapshotNID int64
	stmt := s.selectLatestEventNIDsStmt
	err := stmt.QueryRowContext(ctx, int64(roomNID)).Scan(&nids, &stateSnapshotNID)
	if err != nil {
		return nil, 0, err
	}
	eventNIDs := make([]types.EventNID, len(nids))
	for i := range nids {
		eventNIDs[i] = types.EventNID(nids[i])
	}
	return eventNIDs, types.StateSnapshotNID(stateSnapshotNID), nil
}

func (s *roomStatements) selectLatestEventsNIDsForUpdate(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) ([]types.EventNID, types.EventNID, types.StateSnapshotNID, error) {
	var nids pq.Int64Array
	var lastEventSentNID int64
	var stateSnapshotNID int64
	stmt := common.TxStmt(txn, s.selectLatestEventNIDsForUpdateStmt)
	err := stmt.QueryRowContext(ctx, int64(roomNID)).Scan(&nids, &lastEventSentNID, &stateSnapshotNID)
	if err != nil {
		return nil, 0, 0, err
	}
	eventNIDs := make([]types.EventNID, len(nids))
	for i := range nids {
		eventNIDs[i] = types.EventNID(nids[i])
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
		roomNID,
		eventNIDsAsArray(eventNIDs),
		int64(lastEventSentNID),
		int64(stateSnapshotNID),
	)
	return err
}

func (s *roomStatements) selectRoomVersionForRoomID(
	ctx context.Context, txn *sql.Tx, roomID string,
) (gomatrixserverlib.RoomVersion, error) {
	var roomVersion gomatrixserverlib.RoomVersion
	stmt := common.TxStmt(txn, s.selectRoomVersionForRoomIDStmt)
	err := stmt.QueryRowContext(ctx, roomID).Scan(&roomVersion)
	if err == sql.ErrNoRows {
		debug.PrintStack()
		return roomVersion, fmt.Errorf("room ID %q not found", roomID)
	}
	return roomVersion, err
}

func (s *roomStatements) selectRoomVersionForRoomNID(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) (gomatrixserverlib.RoomVersion, error) {
	var roomVersion gomatrixserverlib.RoomVersion
	stmt := common.TxStmt(txn, s.selectRoomVersionForRoomNIDStmt)
	err := stmt.QueryRowContext(ctx, roomNID).Scan(&roomVersion)
	if err == sql.ErrNoRows {
		debug.PrintStack()
		return roomVersion, fmt.Errorf("room NID %d not found", roomNID)
	}
	return roomVersion, err
}
