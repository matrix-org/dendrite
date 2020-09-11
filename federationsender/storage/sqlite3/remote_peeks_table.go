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

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

const remotePeeksSchema = `
CREATE TABLE IF NOT EXISTS federationsender_remote_peeks (
	room_id TEXT NOT NULL,
	server_name TEXT NOT NULL,
	peek_id TEXT NOT NULL,
    creation_ts INTEGER NOT NULL,
    renewed_ts INTEGER NOT NULL,
    renewal_interval INTEGER NOT NULL,
	UNIQUE (room_id, server_name, peek_id)
);
`

const insertRemotePeekSQL = "" +
	"INSERT INTO federationsender_remote_peeks (room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval) VALUES ($1, $2, $3, $4, $5, $6)"

const selectRemotePeekSQL = "" +
	"SELECT room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval FROM federationsender_remote_peeks WHERE room_id = $1 and server_name = $2 and peek_id = $3"

const selectRemotePeeksSQL = "" +
	"SELECT room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval FROM federationsender_remote_peeks WHERE room_id = $1"

const renewRemotePeekSQL = "" +
	"UPDATE federationsender_remote_peeks SET renewed_ts=$1, renewal_interval=$2 WHERE room_id = $3 and server_name = $4 and peek_id = $5"

const deleteRemotePeekSQL = "" +
	"DELETE FROM federationsender_remote_peeks WHERE room_id = $1 and server_name = $2"

const deleteRemotePeeksSQL = "" +
	"DELETE FROM federationsender_remote_peeks WHERE room_id = $1"

type remotePeeksStatements struct {
	db                    *sql.DB
	insertRemotePeekStmt  *sql.Stmt
	selectRemotePeekStmt  *sql.Stmt
	selectRemotePeeksStmt *sql.Stmt
	renewRemotePeekStmt   *sql.Stmt
	deleteRemotePeekStmt  *sql.Stmt
	deleteRemotePeeksStmt *sql.Stmt
}

func NewSQLiteRemotePeeksTable(db *sql.DB) (s *remotePeeksStatements, err error) {
	s = &remotePeeksStatements{
		db: db,
	}
	_, err = db.Exec(remotePeeksSchema)
	if err != nil {
		return
	}

	if s.insertRemotePeekStmt, err = db.Prepare(insertRemotePeekSQL); err != nil {
		return
	}
	if s.selectRemotePeekStmt, err = db.Prepare(selectRemotePeekSQL); err != nil {
		return
	}
	if s.selectRemotePeeksStmt, err = db.Prepare(selectRemotePeeksSQL); err != nil {
		return
	}
	if s.renewRemotePeekStmt, err = db.Prepare(renewRemotePeekSQL); err != nil {
		return
	}
	if s.deleteRemotePeeksStmt, err = db.Prepare(deleteRemotePeeksSQL); err != nil {
		return
	}
	if s.deleteRemotePeekStmt, err = db.Prepare(deleteRemotePeekSQL); err != nil {
		return
	}
	return
}

func (s *remotePeeksStatements) InsertRemotePeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int,
) (err error) {
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
	stmt := sqlutil.TxStmt(txn, s.insertRemotePeekStmt)
	_, err := stmt.ExecContext(ctx, roomID, serverName, peekID, nowMilli, nowMilli, renewalInterval)
	return
}

func (s *remotePeeksStatements) RenewRemotePeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int,
) (err error) {
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
	_, err := sqlutil.TxStmt(txn, s.renewRemotePeekStmt).ExecContext(ctx, nowMilli, renewalInterval, roomID, serverName, peekID)
	return
}


func (s *remotePeeksStatements) SelectRemotePeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string,
) (remotePeek types.RemotePeek, err error) {
	rows, err := sqlutil.TxStmt(txn, s.selectRemotePeeksStmt).QueryContext(ctx, roomID)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectRemotePeek: rows.close() failed")
	remotePeek := types.RemotePeek{}
	if err = rows.Scan(
		&remotePeek.RoomID,
		&remotePeek.ServerName,
		&remotePeek.PeekID,
		&remotePeek.CreationTimestamp,
		&remotePeek.RenewTimestamp,
		&remotePeek.RenewalInterval,
	); err != nil {
		return
	}
	return remotePeek, rows.Err()
}

func (s *remotePeeksStatements) SelectRemotePeeks(
	ctx context.Context, txn *sql.Tx, roomID string,
) (remotePeeks []types.RemotePeek, err error) {
	rows, err := sqlutil.TxStmt(txn, s.selectRemotePeeksStmt).QueryContext(ctx, roomID)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectRemotePeeks: rows.close() failed")

	for rows.Next() {
		remotePeek := types.RemotePeek{}
		if err = rows.Scan(
			&remotePeek.RoomID,
			&remotePeek.ServerName,
			&remotePeek.PeekID,
			&remotePeek.CreationTimestamp,
			&remotePeek.RenewTimestamp,
			&remotePeek.RenewalInterval,
		); err != nil {
			return
		}
		remotePeeks = append(remotePeeks, remotePeek)
	}

	return remotePeeks, rows.Err()
}

func (s *remotePeeksStatements) DeleteRemotePeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string,
) (err error) {
	_, err := sqlutil.TxStmt(txn, s.deleteRemotePeekStmt).ExecContext(ctx, roomID, serverName, peekID)
	return
}

func (s *remotePeeksStatements) DeleteRemotePeeks(
	ctx context.Context, txn *sql.Tx, roomID string,
) (err error) {
	_, err := sqlutil.TxStmt(txn, s.deleteRemotePeeksStmt).ExecContext(ctx, roomID)
	return
}
