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
	"fmt"
	"strings"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
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
	  ON CONFLICT DO NOTHING
	  RETURNING room_nid;
`

const selectRoomNIDSQL = "" +
	"SELECT room_nid FROM roomserver_rooms WHERE room_id = $1"

const selectLatestEventNIDsSQL = "" +
	"SELECT latest_event_nids, state_snapshot_nid FROM roomserver_rooms WHERE room_nid = $1"

const selectLatestEventNIDsForUpdateSQL = "" +
	"SELECT latest_event_nids, last_event_sent_nid, state_snapshot_nid FROM roomserver_rooms WHERE room_nid = $1"

const updateLatestEventNIDsSQL = "" +
	"UPDATE roomserver_rooms SET latest_event_nids = $1, last_event_sent_nid = $2, state_snapshot_nid = $3 WHERE room_nid = $4"

const selectRoomVersionsForRoomNIDsSQL = "" +
	"SELECT room_nid, room_version FROM roomserver_rooms WHERE room_nid IN ($1)"

const selectRoomInfoSQL = "" +
	"SELECT room_version, room_nid, state_snapshot_nid, latest_event_nids FROM roomserver_rooms WHERE room_id = $1"

const selectRoomIDsSQL = "" +
	"SELECT room_id FROM roomserver_rooms WHERE latest_event_nids != '[]'"

const bulkSelectRoomIDsSQL = "" +
	"SELECT room_id FROM roomserver_rooms WHERE room_nid IN ($1)"

const bulkSelectRoomNIDsSQL = "" +
	"SELECT room_nid FROM roomserver_rooms WHERE room_id IN ($1)"

type roomStatements struct {
	db                                 *sql.DB
	insertRoomNIDStmt                  *sql.Stmt
	selectRoomNIDStmt                  *sql.Stmt
	selectLatestEventNIDsStmt          *sql.Stmt
	selectLatestEventNIDsForUpdateStmt *sql.Stmt
	updateLatestEventNIDsStmt          *sql.Stmt
	//selectRoomVersionForRoomNIDStmt    *sql.Stmt
	selectRoomInfoStmt *sql.Stmt
	selectRoomIDsStmt  *sql.Stmt
}

func createRoomsTable(db *sql.DB) error {
	_, err := db.Exec(roomsSchema)
	return err
}

func prepareRoomsTable(db *sql.DB) (tables.Rooms, error) {
	s := &roomStatements{
		db: db,
	}

	return s, sqlutil.StatementList{
		{&s.insertRoomNIDStmt, insertRoomNIDSQL},
		{&s.selectRoomNIDStmt, selectRoomNIDSQL},
		{&s.selectLatestEventNIDsStmt, selectLatestEventNIDsSQL},
		{&s.selectLatestEventNIDsForUpdateStmt, selectLatestEventNIDsForUpdateSQL},
		{&s.updateLatestEventNIDsStmt, updateLatestEventNIDsSQL},
		//{&s.selectRoomVersionForRoomNIDsStmt, selectRoomVersionForRoomNIDsSQL},
		{&s.selectRoomInfoStmt, selectRoomInfoSQL},
		{&s.selectRoomIDsStmt, selectRoomIDsSQL},
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

func (s *roomStatements) SelectRoomInfo(ctx context.Context, txn *sql.Tx, roomID string) (*types.RoomInfo, error) {
	var info types.RoomInfo
	var latestNIDsJSON string
	stmt := sqlutil.TxStmt(txn, s.selectRoomInfoStmt)
	err := stmt.QueryRowContext(ctx, roomID).Scan(
		&info.RoomVersion, &info.RoomNID, &info.StateSnapshotNID, &latestNIDsJSON,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	var latestNIDs []int64
	if err = json.Unmarshal([]byte(latestNIDsJSON), &latestNIDs); err != nil {
		return nil, err
	}
	info.IsStub = len(latestNIDs) == 0
	return &info, err
}

func (s *roomStatements) InsertRoomNID(
	ctx context.Context, txn *sql.Tx,
	roomID string, roomVersion gomatrixserverlib.RoomVersion,
) (roomNID types.RoomNID, err error) {
	insertStmt := sqlutil.TxStmt(txn, s.insertRoomNIDStmt)
	if err = insertStmt.QueryRowContext(ctx, roomID, roomVersion).Scan(&roomNID); err != nil {
		return 0, fmt.Errorf("resultStmt.QueryRowContext.Scan: %w", err)
	}
	return
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
	var eventNIDs []types.EventNID
	var nidsJSON string
	var stateSnapshotNID int64
	stmt := sqlutil.TxStmt(txn, s.selectLatestEventNIDsStmt)
	err := stmt.QueryRowContext(ctx, int64(roomNID)).Scan(&nidsJSON, &stateSnapshotNID)
	if err != nil {
		return nil, 0, err
	}
	if err := json.Unmarshal([]byte(nidsJSON), &eventNIDs); err != nil {
		return nil, 0, err
	}
	return eventNIDs, types.StateSnapshotNID(stateSnapshotNID), nil
}

func (s *roomStatements) SelectLatestEventsNIDsForUpdate(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) ([]types.EventNID, types.EventNID, types.StateSnapshotNID, error) {
	var eventNIDs []types.EventNID
	var nidsJSON string
	var lastEventSentNID int64
	var stateSnapshotNID int64
	stmt := sqlutil.TxStmt(txn, s.selectLatestEventNIDsForUpdateStmt)
	err := stmt.QueryRowContext(ctx, int64(roomNID)).Scan(&nidsJSON, &lastEventSentNID, &stateSnapshotNID)
	if err != nil {
		return nil, 0, 0, err
	}
	if err := json.Unmarshal([]byte(nidsJSON), &eventNIDs); err != nil {
		return nil, 0, 0, err
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
		eventNIDsAsArray(eventNIDs),
		int64(lastEventSentNID),
		int64(stateSnapshotNID),
		roomNID,
	)
	return err
}

func (s *roomStatements) SelectRoomVersionsForRoomNIDs(
	ctx context.Context, txn *sql.Tx, roomNIDs []types.RoomNID,
) (map[types.RoomNID]gomatrixserverlib.RoomVersion, error) {
	sqlStr := strings.Replace(selectRoomVersionsForRoomNIDsSQL, "($1)", sqlutil.QueryVariadic(len(roomNIDs)), 1)
	sqlPrep, err := s.db.Prepare(sqlStr)
	if err != nil {
		return nil, err
	}
	defer sqlPrep.Close() // nolint:errcheck
	sqlStmt := sqlutil.TxStmt(txn, sqlPrep)
	iRoomNIDs := make([]interface{}, len(roomNIDs))
	for i, v := range roomNIDs {
		iRoomNIDs[i] = v
	}
	rows, err := sqlStmt.QueryContext(ctx, iRoomNIDs...)
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
	iRoomNIDs := make([]interface{}, len(roomNIDs))
	for i, v := range roomNIDs {
		iRoomNIDs[i] = v
	}
	sqlQuery := strings.Replace(bulkSelectRoomIDsSQL, "($1)", sqlutil.QueryVariadic(len(roomNIDs)), 1)
	var rows *sql.Rows
	var err error
	if txn != nil {
		rows, err = txn.QueryContext(ctx, sqlQuery, iRoomNIDs...)
	} else {
		rows, err = s.db.QueryContext(ctx, sqlQuery, iRoomNIDs...)
	}
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
	iRoomIDs := make([]interface{}, len(roomIDs))
	for i, v := range roomIDs {
		iRoomIDs[i] = v
	}
	sqlQuery := strings.Replace(bulkSelectRoomNIDsSQL, "($1)", sqlutil.QueryVariadic(len(roomIDs)), 1)
	var rows *sql.Rows
	var err error
	if txn != nil {
		rows, err = txn.QueryContext(ctx, sqlQuery, iRoomIDs...)
	} else {
		rows, err = s.db.QueryContext(ctx, sqlQuery, iRoomIDs...)
	}
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
