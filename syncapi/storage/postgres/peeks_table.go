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

package postgres

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
	id BIGINT PRIMARY KEY DEFAULT nextval('syncapi_stream_id'),
	room_id TEXT NOT NULL,
	user_id TEXT NOT NULL,
	device_id TEXT NOT NULL,
	new BOOL NOT NULL DEFAULT true,
	deleted BOOL NOT NULL DEFAULT false,
    -- When the peek was created in UNIX epoch ms.
    creation_ts BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS syncapi_peeks_room_id_idx ON syncapi_peeks(room_id);
CREATE INDEX IF NOT EXISTS syncapi_peeks_user_id_device_id_idx ON syncapi_peeks(user_id, device_id);
`

const insertPeekSQL = "" +
	"INSERT INTO syncapi_peeks" +
	" (room_id, user_id, device_id, creation_ts)" +
	" VALUES ($1, $2, $3, $4) RETURNING id"

const deletePeekSQL = "" +
	"UPDATE syncapi_peeks SET deleted=true, id=nextval('syncapi_stream_id') WHERE room_id = $1 AND user_id = $2 AND device_id = $3 RETURNING id"

const deletePeeksSQL = "" +
	"UPDATE syncapi_peeks SET deleted=true, id=nextval('syncapi_stream_id') WHERE room_id = $1 AND user_id = $2 RETURNING id"

const selectPeeksSQL = "" +
	"SELECT room_id, new FROM syncapi_peeks WHERE user_id = $1 AND device_id = $2 AND deleted=false"

const selectPeekingDevicesSQL = "" +
	"SELECT room_id, user_id, device_id FROM syncapi_peeks WHERE deleted=false"

const markPeeksAsOldSQL = "" +
	"UPDATE syncapi_peeks SET id=nextval('syncapi_stream_id'), new=false WHERE user_id = $1 AND device_id = $2 AND deleted=false RETURNING id"

const selectMaxPeekIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_peeks"

type peekStatements struct {
	db                       *sql.DB
	insertPeekStmt           *sql.Stmt
	deletePeekStmt           *sql.Stmt
	deletePeeksStmt          *sql.Stmt
	selectPeeksStmt          *sql.Stmt
	selectPeekingDevicesStmt *sql.Stmt
	markPeeksAsOldStmt       *sql.Stmt
	selectMaxPeekIDStmt      *sql.Stmt
}

func NewPostgresPeeksTable(db *sql.DB) (tables.Peeks, error) {
	_, err := db.Exec(peeksSchema)
	if err != nil {
		return nil, err
	}
	s := &peekStatements{
		db:                 db,
	}
	if s.insertPeekStmt, err = db.Prepare(insertPeekSQL); err != nil {
		return nil, err
	}
	if s.deletePeekStmt, err = db.Prepare(deletePeekSQL); err != nil {
		return nil, err
	}
	if s.deletePeeksStmt, err = db.Prepare(deletePeeksSQL); err != nil {
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
	if s.selectMaxPeekIDStmt, err = db.Prepare(selectMaxPeekIDSQL); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *peekStatements) InsertPeek(
	ctx context.Context, txn *sql.Tx, roomID, userID, deviceID string,
) (streamPos types.StreamPosition, err error) {
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
	stmt := sqlutil.TxStmt(txn, s.insertPeekStmt)
	err = stmt.QueryRowContext(ctx, roomID, userID, deviceID, nowMilli).Scan(&streamPos)
	return
}

func (s *peekStatements) DeletePeek(
	ctx context.Context, txn *sql.Tx, roomID, userID, deviceID string,
) (streamPos types.StreamPosition, err error) {
	stmt := sqlutil.TxStmt(txn, s.deletePeekStmt)
	err = stmt.QueryRowContext(ctx, roomID, userID, deviceID).Scan(&streamPos)
	return
}

func (s *peekStatements) DeletePeeks(
	ctx context.Context, txn *sql.Tx, roomID, userID string,
) (streamPos types.StreamPosition, err error) {
	stmt := sqlutil.TxStmt(txn, s.deletePeeksStmt)
	err = stmt.QueryRowContext(ctx, roomID, userID).Scan(&streamPos)
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

func (s *peekStatements) MarkPeeksAsOld(
	ctx context.Context, txn *sql.Tx, userID, deviceID string,
) (streamPos types.StreamPosition, err error) {
	stmt := sqlutil.TxStmt(txn, s.markPeeksAsOldStmt)
	err = stmt.QueryRowContext(ctx, userID, deviceID).Scan(&streamPos)
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
		devices = append(devices, types.PeekingDevice{UserID: userID, DeviceID: deviceID})
		result[roomID] = devices
	}
	return result, nil
}

func (s *peekStatements) SelectMaxPeekID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64
	stmt := sqlutil.TxStmt(txn, s.selectMaxPeekIDStmt)
	err = stmt.QueryRowContext(ctx).Scan(&nullableID)
	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}
