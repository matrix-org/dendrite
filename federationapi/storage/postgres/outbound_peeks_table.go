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

	"github.com/matrix-org/dendrite/federationapi/types"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

const outboundPeeksSchema = `
CREATE TABLE IF NOT EXISTS federationsender_outbound_peeks (
	room_id TEXT NOT NULL,
	server_name TEXT NOT NULL,
	peek_id TEXT NOT NULL,
    creation_ts BIGINT NOT NULL,
    renewed_ts BIGINT NOT NULL,
    renewal_interval BIGINT NOT NULL,
	UNIQUE (room_id, server_name, peek_id)
);
`

const insertOutboundPeekSQL = "" +
	"INSERT INTO federationsender_outbound_peeks (room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval) VALUES ($1, $2, $3, $4, $5, $6)"

const selectOutboundPeekSQL = "" +
	"SELECT room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval FROM federationsender_outbound_peeks WHERE room_id = $1 and server_name = $2 and peek_id = $3"

const selectOutboundPeeksSQL = "" +
	"SELECT room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval FROM federationsender_outbound_peeks WHERE room_id = $1"

const renewOutboundPeekSQL = "" +
	"UPDATE federationsender_outbound_peeks SET renewed_ts=$1, renewal_interval=$2 WHERE room_id = $3 and server_name = $4 and peek_id = $5"

const deleteOutboundPeekSQL = "" +
	"DELETE FROM federationsender_outbound_peeks WHERE room_id = $1 and server_name = $2"

const deleteOutboundPeeksSQL = "" +
	"DELETE FROM federationsender_outbound_peeks WHERE room_id = $1"

type outboundPeeksStatements struct {
	db                      *sql.DB
	insertOutboundPeekStmt  *sql.Stmt
	selectOutboundPeekStmt  *sql.Stmt
	selectOutboundPeeksStmt *sql.Stmt
	renewOutboundPeekStmt   *sql.Stmt
	deleteOutboundPeekStmt  *sql.Stmt
	deleteOutboundPeeksStmt *sql.Stmt
}

func NewPostgresOutboundPeeksTable(db *sql.DB) (s *outboundPeeksStatements, err error) {
	s = &outboundPeeksStatements{
		db: db,
	}
	_, err = db.Exec(outboundPeeksSchema)
	if err != nil {
		return
	}

	if s.insertOutboundPeekStmt, err = db.Prepare(insertOutboundPeekSQL); err != nil {
		return
	}
	if s.selectOutboundPeekStmt, err = db.Prepare(selectOutboundPeekSQL); err != nil {
		return
	}
	if s.selectOutboundPeeksStmt, err = db.Prepare(selectOutboundPeeksSQL); err != nil {
		return
	}
	if s.renewOutboundPeekStmt, err = db.Prepare(renewOutboundPeekSQL); err != nil {
		return
	}
	if s.deleteOutboundPeeksStmt, err = db.Prepare(deleteOutboundPeeksSQL); err != nil {
		return
	}
	if s.deleteOutboundPeekStmt, err = db.Prepare(deleteOutboundPeekSQL); err != nil {
		return
	}
	return
}

func (s *outboundPeeksStatements) InsertOutboundPeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64,
) (err error) {
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
	stmt := sqlutil.TxStmt(txn, s.insertOutboundPeekStmt)
	_, err = stmt.ExecContext(ctx, roomID, serverName, peekID, nowMilli, nowMilli, renewalInterval)
	return
}

func (s *outboundPeeksStatements) RenewOutboundPeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64,
) (err error) {
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
	_, err = sqlutil.TxStmt(txn, s.renewOutboundPeekStmt).ExecContext(ctx, nowMilli, renewalInterval, roomID, serverName, peekID)
	return
}

func (s *outboundPeeksStatements) SelectOutboundPeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string,
) (*types.OutboundPeek, error) {
	row := sqlutil.TxStmt(txn, s.selectOutboundPeeksStmt).QueryRowContext(ctx, roomID)
	outboundPeek := types.OutboundPeek{}
	err := row.Scan(
		&outboundPeek.RoomID,
		&outboundPeek.ServerName,
		&outboundPeek.PeekID,
		&outboundPeek.CreationTimestamp,
		&outboundPeek.RenewedTimestamp,
		&outboundPeek.RenewalInterval,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &outboundPeek, nil
}

func (s *outboundPeeksStatements) SelectOutboundPeeks(
	ctx context.Context, txn *sql.Tx, roomID string,
) (outboundPeeks []types.OutboundPeek, err error) {
	rows, err := sqlutil.TxStmt(txn, s.selectOutboundPeeksStmt).QueryContext(ctx, roomID)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectOutboundPeeks: rows.close() failed")

	for rows.Next() {
		outboundPeek := types.OutboundPeek{}
		if err = rows.Scan(
			&outboundPeek.RoomID,
			&outboundPeek.ServerName,
			&outboundPeek.PeekID,
			&outboundPeek.CreationTimestamp,
			&outboundPeek.RenewedTimestamp,
			&outboundPeek.RenewalInterval,
		); err != nil {
			return
		}
		outboundPeeks = append(outboundPeeks, outboundPeek)
	}

	return outboundPeeks, rows.Err()
}

func (s *outboundPeeksStatements) DeleteOutboundPeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string,
) (err error) {
	_, err = sqlutil.TxStmt(txn, s.deleteOutboundPeekStmt).ExecContext(ctx, roomID, serverName, peekID)
	return
}

func (s *outboundPeeksStatements) DeleteOutboundPeeks(
	ctx context.Context, txn *sql.Tx, roomID string,
) (err error) {
	_, err = sqlutil.TxStmt(txn, s.deleteOutboundPeeksStmt).ExecContext(ctx, roomID)
	return
}
