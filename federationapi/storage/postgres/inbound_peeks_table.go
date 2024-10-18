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

const inboundPeeksSchema = `
CREATE TABLE IF NOT EXISTS federationsender_inbound_peeks (
	room_id TEXT NOT NULL,
	server_name TEXT NOT NULL,
	peek_id TEXT NOT NULL,
    creation_ts BIGINT NOT NULL,
    renewed_ts BIGINT NOT NULL,
    renewal_interval BIGINT NOT NULL,
	UNIQUE (room_id, server_name, peek_id)
);
`

const insertInboundPeekSQL = "" +
	"INSERT INTO federationsender_inbound_peeks (room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval) VALUES ($1, $2, $3, $4, $5, $6)"

const selectInboundPeekSQL = "" +
	"SELECT room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval FROM federationsender_inbound_peeks WHERE room_id = $1 and server_name = $2 and peek_id = $3"

const selectInboundPeeksSQL = "" +
	"SELECT room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval FROM federationsender_inbound_peeks WHERE room_id = $1 ORDER by creation_ts"

const renewInboundPeekSQL = "" +
	"UPDATE federationsender_inbound_peeks SET renewed_ts=$1, renewal_interval=$2 WHERE room_id = $3 and server_name = $4 and peek_id = $5"

const deleteInboundPeekSQL = "" +
	"DELETE FROM federationsender_inbound_peeks WHERE room_id = $1 and server_name = $2 and peek_id = $3"

const deleteInboundPeeksSQL = "" +
	"DELETE FROM federationsender_inbound_peeks WHERE room_id = $1"

type inboundPeeksStatements struct {
	db                     *sql.DB
	insertInboundPeekStmt  *sql.Stmt
	selectInboundPeekStmt  *sql.Stmt
	selectInboundPeeksStmt *sql.Stmt
	renewInboundPeekStmt   *sql.Stmt
	deleteInboundPeekStmt  *sql.Stmt
	deleteInboundPeeksStmt *sql.Stmt
}

func NewPostgresInboundPeeksTable(db *sql.DB) (s *inboundPeeksStatements, err error) {
	s = &inboundPeeksStatements{
		db: db,
	}
	_, err = db.Exec(inboundPeeksSchema)
	if err != nil {
		return
	}

	return s, sqlutil.StatementList{
		{&s.insertInboundPeekStmt, insertInboundPeekSQL},
		{&s.selectInboundPeekStmt, selectInboundPeekSQL},
		{&s.selectInboundPeekStmt, selectInboundPeekSQL},
		{&s.selectInboundPeeksStmt, selectInboundPeeksSQL},
		{&s.renewInboundPeekStmt, renewInboundPeekSQL},
		{&s.deleteInboundPeeksStmt, deleteInboundPeeksSQL},
		{&s.deleteInboundPeekStmt, deleteInboundPeekSQL},
	}.Prepare(db)
}

func (s *inboundPeeksStatements) InsertInboundPeek(
	ctx context.Context, txn *sql.Tx, serverName spec.ServerName, roomID, peekID string, renewalInterval int64,
) (err error) {
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
	stmt := sqlutil.TxStmt(txn, s.insertInboundPeekStmt)
	_, err = stmt.ExecContext(ctx, roomID, serverName, peekID, nowMilli, nowMilli, renewalInterval)
	return
}

func (s *inboundPeeksStatements) RenewInboundPeek(
	ctx context.Context, txn *sql.Tx, serverName spec.ServerName, roomID, peekID string, renewalInterval int64,
) (err error) {
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
	_, err = sqlutil.TxStmt(txn, s.renewInboundPeekStmt).ExecContext(ctx, nowMilli, renewalInterval, roomID, serverName, peekID)
	return
}

func (s *inboundPeeksStatements) SelectInboundPeek(
	ctx context.Context, txn *sql.Tx, serverName spec.ServerName, roomID, peekID string,
) (*types.InboundPeek, error) {
	row := sqlutil.TxStmt(txn, s.selectInboundPeeksStmt).QueryRowContext(ctx, roomID)
	inboundPeek := types.InboundPeek{}
	err := row.Scan(
		&inboundPeek.RoomID,
		&inboundPeek.ServerName,
		&inboundPeek.PeekID,
		&inboundPeek.CreationTimestamp,
		&inboundPeek.RenewedTimestamp,
		&inboundPeek.RenewalInterval,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inboundPeek, nil
}

func (s *inboundPeeksStatements) SelectInboundPeeks(
	ctx context.Context, txn *sql.Tx, roomID string,
) (inboundPeeks []types.InboundPeek, err error) {
	rows, err := sqlutil.TxStmt(txn, s.selectInboundPeeksStmt).QueryContext(ctx, roomID)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectInboundPeeks: rows.close() failed")

	for rows.Next() {
		inboundPeek := types.InboundPeek{}
		if err = rows.Scan(
			&inboundPeek.RoomID,
			&inboundPeek.ServerName,
			&inboundPeek.PeekID,
			&inboundPeek.CreationTimestamp,
			&inboundPeek.RenewedTimestamp,
			&inboundPeek.RenewalInterval,
		); err != nil {
			return
		}
		inboundPeeks = append(inboundPeeks, inboundPeek)
	}

	return inboundPeeks, rows.Err()
}

func (s *inboundPeeksStatements) DeleteInboundPeek(
	ctx context.Context, txn *sql.Tx, serverName spec.ServerName, roomID, peekID string,
) (err error) {
	_, err = sqlutil.TxStmt(txn, s.deleteInboundPeekStmt).ExecContext(ctx, roomID, serverName, peekID)
	return
}

func (s *inboundPeeksStatements) DeleteInboundPeeks(
	ctx context.Context, txn *sql.Tx, roomID string,
) (err error) {
	_, err = sqlutil.TxStmt(txn, s.deleteInboundPeeksStmt).ExecContext(ctx, roomID)
	return
}
