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
	"fmt"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const roomsSchema = `
  CREATE TABLE IF NOT EXISTS roomserver_rooms (
    room_nid INTEGER PRIMARY KEY AUTOINCREMENT,
    room_id TEXT NOT NULL UNIQUE,
    latest_event_nids TEXT NOT NULL DEFAULT '{}',
    last_event_sent_nid INTEGER NOT NULL DEFAULT 0,
    state_snapshot_nid INTEGER NOT NULL DEFAULT 0
  );
`

// Same as insertEventTypeNIDSQL
const insertRoomNIDSQL = `
	INSERT INTO roomserver_rooms (room_id) VALUES ($1)
	  ON CONFLICT DO NOTHING;
`

const insertRoomNIDResultSQL = `
	SELECT room_nid FROM roomserver_rooms
		WHERE rowid = last_insert_rowid();
`

const selectRoomNIDSQL = "" +
	"SELECT room_nid FROM roomserver_rooms WHERE room_id = $1"

const selectLatestEventNIDsSQL = "" +
	"SELECT latest_event_nids, state_snapshot_nid FROM roomserver_rooms WHERE room_nid = $1"

const selectLatestEventNIDsForUpdateSQL = "" +
	"SELECT latest_event_nids, last_event_sent_nid, state_snapshot_nid FROM roomserver_rooms WHERE room_nid = $1"

const updateLatestEventNIDsSQL = "" +
	"UPDATE roomserver_rooms SET latest_event_nids = $2, last_event_sent_nid = $3, state_snapshot_nid = $4 WHERE room_nid = $1"

type roomStatements struct {
	insertRoomNIDStmt                  *sql.Stmt
	insertRoomNIDResultStmt            *sql.Stmt
	selectRoomNIDStmt                  *sql.Stmt
	selectLatestEventNIDsStmt          *sql.Stmt
	selectLatestEventNIDsForUpdateStmt *sql.Stmt
	updateLatestEventNIDsStmt          *sql.Stmt
}

func (s *roomStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(roomsSchema)
	if err != nil {
		return
	}
	return statementList{
		{&s.insertRoomNIDStmt, insertRoomNIDSQL},
		{&s.insertRoomNIDResultStmt, insertRoomNIDResultSQL},
		{&s.selectRoomNIDStmt, selectRoomNIDSQL},
		{&s.selectLatestEventNIDsStmt, selectLatestEventNIDsSQL},
		{&s.selectLatestEventNIDsForUpdateStmt, selectLatestEventNIDsForUpdateSQL},
		{&s.updateLatestEventNIDsStmt, updateLatestEventNIDsSQL},
	}.prepare(db)
}

func (s *roomStatements) insertRoomNID(
	ctx context.Context, txn *sql.Tx, roomID string,
) (types.RoomNID, error) {
	var roomNID int64
	var err error
	insertStmt := common.TxStmt(txn, s.insertRoomNIDStmt)
	resultStmt := common.TxStmt(txn, s.insertRoomNIDResultStmt)
	if _, err = insertStmt.ExecContext(ctx, roomID); err == nil {
		err = resultStmt.QueryRowContext(ctx).Scan(&roomNID)
		if err != nil {
			fmt.Println("insertRoomNID resultStmt.QueryRowContext:", err)
		}
	} else {
		fmt.Println("insertRoomNID insertStmt.ExecContext:", err)
	}
	return types.RoomNID(roomNID), err
}

func (s *roomStatements) selectRoomNID(
	ctx context.Context, txn *sql.Tx, roomID string,
) (types.RoomNID, error) {
	var roomNID int64
	stmt := common.TxStmt(txn, s.selectRoomNIDStmt)
	err := stmt.QueryRowContext(ctx, roomID).Scan(&roomNID)
	if err != nil {
		fmt.Println("selectRoomNID stmt.QueryRowContext:", err)
	}
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
		fmt.Println("selectLatestEventNIDs stmt.QueryRowContext:", err)
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
		fmt.Println("selectLatestEventsNIDsForUpdate stmt.QueryRowContext:", err)
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
	if err != nil {
		fmt.Println("updateLatestEventNIDs stmt.ExecContext:", err)
	}
	return err
}
