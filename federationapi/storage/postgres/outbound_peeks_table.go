// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/element-hq/dendrite/federationapi/types"
	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib/spec"
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
	"SELECT room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval FROM federationsender_outbound_peeks WHERE room_id = $1 ORDER BY creation_ts"

const renewOutboundPeekSQL = "" +
	"UPDATE federationsender_outbound_peeks SET renewed_ts=$1, renewal_interval=$2 WHERE room_id = $3 and server_name = $4 and peek_id = $5"

const deleteOutboundPeekSQL = "" +
	"DELETE FROM federationsender_outbound_peeks WHERE room_id = $1 and server_name = $2 and peek_id = $3"

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

	return s, sqlutil.StatementList{
		{&s.insertOutboundPeekStmt, insertOutboundPeekSQL},
		{&s.selectOutboundPeekStmt, selectOutboundPeekSQL},
		{&s.selectOutboundPeeksStmt, selectOutboundPeeksSQL},
		{&s.renewOutboundPeekStmt, renewOutboundPeekSQL},
		{&s.deleteOutboundPeeksStmt, deleteOutboundPeeksSQL},
		{&s.deleteOutboundPeekStmt, deleteOutboundPeekSQL},
	}.Prepare(db)
}

func (s *outboundPeeksStatements) InsertOutboundPeek(
	ctx context.Context, txn *sql.Tx, serverName spec.ServerName, roomID, peekID string, renewalInterval int64,
) (err error) {
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
	stmt := sqlutil.TxStmt(txn, s.insertOutboundPeekStmt)
	_, err = stmt.ExecContext(ctx, roomID, serverName, peekID, nowMilli, nowMilli, renewalInterval)
	return
}

func (s *outboundPeeksStatements) RenewOutboundPeek(
	ctx context.Context, txn *sql.Tx, serverName spec.ServerName, roomID, peekID string, renewalInterval int64,
) (err error) {
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
	_, err = sqlutil.TxStmt(txn, s.renewOutboundPeekStmt).ExecContext(ctx, nowMilli, renewalInterval, roomID, serverName, peekID)
	return
}

func (s *outboundPeeksStatements) SelectOutboundPeek(
	ctx context.Context, txn *sql.Tx, serverName spec.ServerName, roomID, peekID string,
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
	ctx context.Context, txn *sql.Tx, serverName spec.ServerName, roomID, peekID string,
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
