// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"time"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
)

const peeksSchema = `
CREATE TABLE IF NOT EXISTS syncapi_peeks (
	id INTEGER PRIMARY KEY,
	room_id TEXT NOT NULL,
	user_id TEXT NOT NULL,
	device_id TEXT NOT NULL,
	new BOOL NOT NULL DEFAULT true,
    -- When the peek was created in UNIX epoch ms.
    creation_ts INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS syncapi_peeks_room_id_idx ON syncapi_peeks(room_id);
CREATE INDEX IF NOT EXISTS syncapi_peeks_user_id_device_id_idx ON syncapi_peeks(user_id, device_id);
`

const insertPeekSQL = "" +
	"INSERT INTO syncapi_peeks" +
	" (id, room_id, user_id, device_id, creation_ts)" +
	" VALUES ($1, $2, $3, $4, $5)"

const deletePeekSQL = "" +
	"DELETE FROM syncapi_peeks WHERE room_id = $1 AND user_id = $2 and device_id = $3"

const selectPeeksSQL = "" +
	"SELECT room_id, new FROM syncapi_peeks WHERE user_id = $1 and device_id = $2"

const selectPeekingDevicesSQL = "" +
	"SELECT room_id, user_id, device_id FROM syncapi_peeks"

const markPeeksAsOldSQL = "" +
	"UPDATE syncapi_peeks SET new=false WHERE user_id = $1 and device_id = $2"

type peekStatements struct {
	db                            *sql.DB
	streamIDStatements            *streamIDStatements
	insertPeekStmt        		  *sql.Stmt
	deletePeekStmt         		  *sql.Stmt
	selectPeeksStmt				  *sql.Stmt
	selectPeekingDevicesStmt	  *sql.Stmt
	markPeeksAsOldStmt            *sql.Stmt
}

func NewSqlitePeeksTable(db *sql.DB, streamID *streamIDStatements) (tables.Peeks, error) {
	_, err := db.Exec(peeksSchema)
	if err != nil {
		return nil, err
	}
	s := &peekStatements{
		db: db,
		streamIDStatements: streamID,
	}
	if s.insertPeekStmt, err = db.Prepare(insertPeekSQL); err != nil {
		return nil, err
	}
	if s.deletePeekStmt, err = db.Prepare(deletePeekSQL); err != nil {
		return nil, err
	}
	if s.selectPeeksStmt, err = db.Prepare(selectPeeksSQL); err != nil {
		return nil, err
	}
	if s.selectPeekingDevicesStmt, err = db.Prepare(selectPeekingDevicesSQL); err != nil {
		return nil, err
	}
	if s.markPeeksAsOldStmt, err = db.Prepare(markPeeksAsOldSQL); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *peekStatements) InsertPeek(
	ctx context.Context, txn *sql.Tx, roomID, userID, deviceID string,
) (streamPos types.StreamPosition, err error) {
	streamPos, err = s.streamIDStatements.nextStreamID(ctx, txn)
	if err != nil {
		return
	}
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
	_, err = sqlutil.TxStmt(txn, s.insertPeekStmt).ExecContext(ctx, roomID, userID, deviceID, nowMilli)
	return
}

func (s *peekStatements) DeletePeek(
	ctx context.Context, txn *sql.Tx, roomID, userID, deviceID string,
) (streamPos types.StreamPosition, err error) {
	streamPos, err = s.streamIDStatements.nextStreamID(ctx, txn)
	if err != nil {
		return
	}
	_, err = sqlutil.TxStmt(txn, s.deletePeekStmt).ExecContext(ctx, roomID, userID, deviceID)
	return
}

func (s *peekStatements) SelectPeeks(
	ctx context.Context, txn *sql.Tx, userID, deviceID string,
) (peeks []types.Peek, err error) {
	rows, err := sqlutil.TxStmt(txn, s.selectPeeksStmt).QueryContext(ctx, userID, deviceID)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectPeeks: rows.close() failed")

	for rows.Next() {
		peek := types.Peek{}
		if err = rows.Scan(&peek.RoomID, &peek.New); err != nil {
			return
		}
		peeks = append(peeks, peek)
	}

	return peeks, rows.Err()
}

func (s *peekStatements) MarkPeeksAsOld (
	ctx context.Context, txn *sql.Tx, userID, deviceID string,
) (err error) {
	_, err = sqlutil.TxStmt(txn, s.markPeeksAsOldStmt).ExecContext(ctx, userID, deviceID)
	return
}

func (s *peekStatements) SelectPeekingDevices(
	ctx context.Context,
) (peekingDevices map[string][]types.PeekingDevice, err error) {
	rows, err := s.selectPeekingDevicesStmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectPeekingDevices: rows.close() failed")

	result := make(map[string][]types.PeekingDevice)
	for rows.Next() {
		var roomID, userID, deviceID string
		if err := rows.Scan(&roomID, &userID, &deviceID); err != nil {
			return nil, err
		}
		devices := result[roomID]
		devices = append(devices, types.PeekingDevice{userID, deviceID})
		result[roomID] = devices
	}
	return result, nil
}
