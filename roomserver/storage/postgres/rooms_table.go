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

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
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

const selectRoomVersionsForRoomNIDsSQL = "" +
	"SELECT room_nid, room_version FROM roomserver_rooms WHERE room_nid = ANY($1)"

const selectRoomInfoSQL = "" +
	"SELECT room_version, room_nid, state_snapshot_nid, latest_event_nids FROM roomserver_rooms WHERE room_id = $1"

const selectRoomIDsSQL = "" +
	"SELECT room_id FROM roomserver_rooms WHERE array_length(latest_event_nids, 1) > 0"

const bulkSelectRoomIDsSQL = "" +
	"SELECT room_id FROM roomserver_rooms WHERE room_nid = ANY($1)"

const bulkSelectRoomNIDsSQL = "" +
	"SELECT room_nid FROM roomserver_rooms WHERE room_id = ANY($1)"

type roomStatements struct {
	insertRoomNIDStmt                  *sql.Stmt
	selectRoomNIDStmt                  *sql.Stmt
	selectLatestEventNIDsStmt          *sql.Stmt
	selectLatestEventNIDsForUpdateStmt *sql.Stmt
	updateLatestEventNIDsStmt          *sql.Stmt
	selectRoomVersionsForRoomNIDsStmt  *sql.Stmt
	selectRoomInfoStmt                 *sql.Stmt
	selectRoomIDsStmt                  *sql.Stmt
	bulkSelectRoomIDsStmt              *sql.Stmt
	bulkSelectRoomNIDsStmt             *sql.Stmt
}

func createRoomsTable(db *sql.DB) error {
	_, err := db.Exec(roomsSchema)
	return err
}

func prepareRoomsTable(db *sql.DB) (tables.Rooms, error) {
	s := &roomStatements{}

	return s, sqlutil.StatementList{
		{&s.insertRoomNIDStmt, insertRoomNIDSQL},
		{&s.selectRoomNIDStmt, selectRoomNIDSQL},
		{&s.selectLatestEventNIDsStmt, selectLatestEventNIDsSQL},
		{&s.selectLatestEventNIDsForUpdateStmt, selectLatestEventNIDsForUpdateSQL},
		{&s.updateLatestEventNIDsStmt, updateLatestEventNIDsSQL},
		{&s.selectRoomVersionsForRoomNIDsStmt, selectRoomVersionsForRoomNIDsSQL},
		{&s.selectRoomInfoStmt, selectRoomInfoSQL},
		{&s.selectRoomIDsStmt, selectRoomIDsSQL},
		{&s.bulkSelectRoomIDsStmt, bulkSelectRoomIDsSQL},
		{&s.bulkSelectRoomNIDsStmt, bulkSelectRoomNIDsSQL},
	}.Prepare(db)
}

func (s *roomStatements) SelectRoomIDs(ctx context.Context, txn *sql.Tx) ([]string, error) {
	stmt := sqlutil.TxStmt(txn, s.selectRoomIDsStmt)
	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectRoomIDsStmt: rows.close() failed")
	var roomIDs []string
	for rows.Next() {
		var roomID string
		if err = rows.Scan(&roomID); err != nil {
			return nil, err
		}
		roomIDs = append(roomIDs, roomID)
	}
	return roomIDs, nil
}
func (s *roomStatements) InsertRoomNID(
	ctx context.Context, txn *sql.Tx,
	roomID string, roomVersion gomatrixserverlib.RoomVersion,
) (types.RoomNID, error) {
	var roomNID int64
	stmt := sqlutil.TxStmt(txn, s.insertRoomNIDStmt)
	err := stmt.QueryRowContext(ctx, roomID, roomVersion).Scan(&roomNID)
	return types.RoomNID(roomNID), err
}

func (s *roomStatements) SelectRoomInfo(ctx context.Context, txn *sql.Tx, roomID string) (*types.RoomInfo, error) {
	var info types.RoomInfo
	var latestNIDs pq.Int64Array
	stmt := sqlutil.TxStmt(txn, s.selectRoomInfoStmt)
	err := stmt.QueryRowContext(ctx, roomID).Scan(
		&info.RoomVersion, &info.RoomNID, &info.StateSnapshotNID, &latestNIDs,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	info.IsStub = len(latestNIDs) == 0
	return &info, err
}

func (s *roomStatements) SelectRoomNID(
	ctx context.Context, txn *sql.Tx, roomID string,
) (types.RoomNID, error) {
	var roomNID int64
	stmt := sqlutil.TxStmt(txn, s.selectRoomNIDStmt)
	err := stmt.QueryRowContext(ctx, roomID).Scan(&roomNID)
	return types.RoomNID(roomNID), err
}

func (s *roomStatements) SelectLatestEventNIDs(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) ([]types.EventNID, types.StateSnapshotNID, error) {
	var nids pq.Int64Array
	var stateSnapshotNID int64
	stmt := sqlutil.TxStmt(txn, s.selectLatestEventNIDsStmt)
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

func (s *roomStatements) SelectLatestEventsNIDsForUpdate(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) ([]types.EventNID, types.EventNID, types.StateSnapshotNID, error) {
	var nids pq.Int64Array
	var lastEventSentNID int64
	var stateSnapshotNID int64
	stmt := sqlutil.TxStmt(txn, s.selectLatestEventNIDsForUpdateStmt)
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

func (s *roomStatements) UpdateLatestEventNIDs(
	ctx context.Context,
	txn *sql.Tx,
	roomNID types.RoomNID,
	eventNIDs []types.EventNID,
	lastEventSentNID types.EventNID,
	stateSnapshotNID types.StateSnapshotNID,
) error {
	stmt := sqlutil.TxStmt(txn, s.updateLatestEventNIDsStmt)
	_, err := stmt.ExecContext(
		ctx,
		roomNID,
		eventNIDsAsArray(eventNIDs),
		int64(lastEventSentNID),
		int64(stateSnapshotNID),
	)
	return err
}

func (s *roomStatements) SelectRoomVersionsForRoomNIDs(
	ctx context.Context, txn *sql.Tx, roomNIDs []types.RoomNID,
) (map[types.RoomNID]gomatrixserverlib.RoomVersion, error) {
	stmt := sqlutil.TxStmt(txn, s.selectRoomVersionsForRoomNIDsStmt)
	rows, err := stmt.QueryContext(ctx, roomNIDsAsArray(roomNIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectRoomVersionsForRoomNIDsStmt: rows.close() failed")
	result := make(map[types.RoomNID]gomatrixserverlib.RoomVersion)
	for rows.Next() {
		var roomNID types.RoomNID
		var roomVersion gomatrixserverlib.RoomVersion
		if err = rows.Scan(&roomNID, &roomVersion); err != nil {
			return nil, err
		}
		result[roomNID] = roomVersion
	}
	return result, nil
}

func (s *roomStatements) BulkSelectRoomIDs(ctx context.Context, txn *sql.Tx, roomNIDs []types.RoomNID) ([]string, error) {
	var array pq.Int64Array
	for _, nid := range roomNIDs {
		array = append(array, int64(nid))
	}
	stmt := sqlutil.TxStmt(txn, s.bulkSelectRoomIDsStmt)
	rows, err := stmt.QueryContext(ctx, array)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectRoomIDsStmt: rows.close() failed")
	var roomIDs []string
	for rows.Next() {
		var roomID string
		if err = rows.Scan(&roomID); err != nil {
			return nil, err
		}
		roomIDs = append(roomIDs, roomID)
	}
	return roomIDs, nil
}

func (s *roomStatements) BulkSelectRoomNIDs(ctx context.Context, txn *sql.Tx, roomIDs []string) ([]types.RoomNID, error) {
	var array pq.StringArray
	for _, roomID := range roomIDs {
		array = append(array, roomID)
	}
	stmt := sqlutil.TxStmt(txn, s.bulkSelectRoomNIDsStmt)
	rows, err := stmt.QueryContext(ctx, array)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectRoomNIDsStmt: rows.close() failed")
	var roomNIDs []types.RoomNID
	for rows.Next() {
		var roomNID types.RoomNID
		if err = rows.Scan(&roomNID); err != nil {
			return nil, err
		}
		roomNIDs = append(roomNIDs, roomNID)
	}
	return roomNIDs, nil
}

func roomNIDsAsArray(roomNIDs []types.RoomNID) pq.Int64Array {
	nids := make([]int64, len(roomNIDs))
	for i := range roomNIDs {
		nids[i] = int64(roomNIDs[i])
	}
	return nids
}
