// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package sqlite3

import (
	"context"
	"database/sql"
	"time"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/syncapi/storage/tables"
	"github.com/element-hq/dendrite/syncapi/types"
)

const peeksSchema = `
CREATE TABLE IF NOT EXISTS syncapi_peeks (
	id INTEGER,
	room_id TEXT NOT NULL,
	user_id TEXT NOT NULL,
	device_id TEXT NOT NULL,
	deleted BOOL NOT NULL DEFAULT false,
    -- When the peek was created in UNIX epoch ms.
    creation_ts INTEGER NOT NULL,
    UNIQUE(room_id, user_id, device_id)
);

CREATE INDEX IF NOT EXISTS syncapi_peeks_room_id_idx ON syncapi_peeks(room_id);
CREATE INDEX IF NOT EXISTS syncapi_peeks_user_id_device_id_idx ON syncapi_peeks(user_id, device_id);
`

const insertPeekSQL = "" +
	"INSERT OR REPLACE INTO syncapi_peeks" +
	" (id, room_id, user_id, device_id, creation_ts, deleted)" +
	" VALUES ($1, $2, $3, $4, $5, false)"

const deletePeekSQL = "" +
	"UPDATE syncapi_peeks SET deleted=true, id=$1 WHERE room_id = $2 AND user_id = $3 AND device_id = $4"

const deletePeeksSQL = "" +
	"UPDATE syncapi_peeks SET deleted=true, id=$1 WHERE room_id = $2 AND user_id = $3"

// we care about all the peeks which were created in this range, deleted in this range,
// or were created before this range but haven't been deleted yet.
// BEWARE: sqlite chokes on out of order substitution strings.
const selectPeeksInRangeSQL = "" +
	"SELECT id, room_id, deleted FROM syncapi_peeks WHERE user_id = $1 AND device_id = $2 AND ((id <= $3 AND NOT deleted=true) OR (id > $3 AND id <= $4))"

const selectPeekingDevicesSQL = "" +
	"SELECT room_id, user_id, device_id FROM syncapi_peeks WHERE deleted=false"

const selectMaxPeekIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_peeks"

const purgePeeksSQL = "" +
	"DELETE FROM syncapi_peeks WHERE room_id = $1"

type peekStatements struct {
	db                       *sql.DB
	streamIDStatements       *StreamIDStatements
	insertPeekStmt           *sql.Stmt
	deletePeekStmt           *sql.Stmt
	deletePeeksStmt          *sql.Stmt
	selectPeeksInRangeStmt   *sql.Stmt
	selectPeekingDevicesStmt *sql.Stmt
	selectMaxPeekIDStmt      *sql.Stmt
	purgePeeksStmt           *sql.Stmt
}

func NewSqlitePeeksTable(db *sql.DB, streamID *StreamIDStatements) (tables.Peeks, error) {
	_, err := db.Exec(peeksSchema)
	if err != nil {
		return nil, err
	}
	s := &peekStatements{
		db:                 db,
		streamIDStatements: streamID,
	}
	return s, sqlutil.StatementList{
		{&s.insertPeekStmt, insertPeekSQL},
		{&s.deletePeekStmt, deletePeekSQL},
		{&s.deletePeeksStmt, deletePeeksSQL},
		{&s.selectPeeksInRangeStmt, selectPeeksInRangeSQL},
		{&s.selectPeekingDevicesStmt, selectPeekingDevicesSQL},
		{&s.selectMaxPeekIDStmt, selectMaxPeekIDSQL},
		{&s.purgePeeksStmt, purgePeeksSQL},
	}.Prepare(db)
}

func (s *peekStatements) InsertPeek(
	ctx context.Context, txn *sql.Tx, roomID, userID, deviceID string,
) (streamPos types.StreamPosition, err error) {
	streamPos, err = s.streamIDStatements.nextPDUID(ctx, txn)
	if err != nil {
		return
	}
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
	_, err = sqlutil.TxStmt(txn, s.insertPeekStmt).ExecContext(ctx, streamPos, roomID, userID, deviceID, nowMilli)
	return
}

func (s *peekStatements) DeletePeek(
	ctx context.Context, txn *sql.Tx, roomID, userID, deviceID string,
) (streamPos types.StreamPosition, err error) {
	streamPos, err = s.streamIDStatements.nextPDUID(ctx, txn)
	if err != nil {
		return
	}
	_, err = sqlutil.TxStmt(txn, s.deletePeekStmt).ExecContext(ctx, streamPos, roomID, userID, deviceID)
	return
}

func (s *peekStatements) DeletePeeks(
	ctx context.Context, txn *sql.Tx, roomID, userID string,
) (types.StreamPosition, error) {
	streamPos, err := s.streamIDStatements.nextPDUID(ctx, txn)
	if err != nil {
		return 0, err
	}
	result, err := sqlutil.TxStmt(txn, s.deletePeeksStmt).ExecContext(ctx, streamPos, roomID, userID)
	if err != nil {
		return 0, err
	}
	numAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if numAffected == 0 {
		return 0, sql.ErrNoRows
	}
	return streamPos, nil
}

func (s *peekStatements) SelectPeeksInRange(
	ctx context.Context, txn *sql.Tx, userID, deviceID string, r types.Range,
) (peeks []types.Peek, err error) {
	rows, err := sqlutil.TxStmt(txn, s.selectPeeksInRangeStmt).QueryContext(ctx, userID, deviceID, r.Low(), r.High())
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectPeeksInRange: rows.close() failed")

	for rows.Next() {
		peek := types.Peek{}
		var id types.StreamPosition
		if err = rows.Scan(&id, &peek.RoomID, &peek.Deleted); err != nil {
			return
		}
		peek.New = (id > r.Low() && id <= r.High()) && !peek.Deleted
		peeks = append(peeks, peek)
	}

	return peeks, rows.Err()
}

func (s *peekStatements) SelectPeekingDevices(
	ctx context.Context, txn *sql.Tx,
) (peekingDevices map[string][]types.PeekingDevice, err error) {
	rows, err := sqlutil.TxStmt(txn, s.selectPeekingDevicesStmt).QueryContext(ctx)
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
	return result, rows.Err()
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

func (s *peekStatements) PurgePeeks(
	ctx context.Context, txn *sql.Tx, roomID string,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgePeeksStmt).ExecContext(ctx, roomID)
	return err
}
