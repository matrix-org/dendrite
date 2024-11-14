// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/roomserver/storage/postgres/deltas"
	"github.com/element-hq/dendrite/roomserver/storage/tables"
)

const publishedSchema = `
-- Stores which rooms are published in the room directory
CREATE TABLE IF NOT EXISTS roomserver_published (
    -- The room ID of the room
    room_id TEXT NOT NULL,
    -- The appservice ID of the room
    appservice_id TEXT NOT NULL,
    -- The network_id of the room
    network_id TEXT NOT NULL,
    -- Whether it is published or not
    published BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (room_id, appservice_id, network_id)
);
`

const upsertPublishedSQL = "" +
	"INSERT INTO roomserver_published (room_id, appservice_id, network_id, published) VALUES ($1, $2, $3, $4) " +
	"ON CONFLICT (room_id, appservice_id, network_id) DO UPDATE SET published=$4"

const selectAllPublishedSQL = "" +
	"SELECT room_id FROM roomserver_published WHERE published = $1 AND CASE WHEN $2 THEN 1=1 ELSE network_id = '' END ORDER BY room_id ASC"

const selectNetworkPublishedSQL = "" +
	"SELECT room_id FROM roomserver_published WHERE published = $1 AND network_id = $2 ORDER BY room_id ASC"

const selectPublishedSQL = "" +
	"SELECT published FROM roomserver_published WHERE room_id = $1"

type publishedStatements struct {
	upsertPublishedStmt        *sql.Stmt
	selectAllPublishedStmt     *sql.Stmt
	selectPublishedStmt        *sql.Stmt
	selectNetworkPublishedStmt *sql.Stmt
}

func CreatePublishedTable(db *sql.DB) error {
	_, err := db.Exec(publishedSchema)
	if err != nil {
		return err
	}
	m := sqlutil.NewMigrator(db)
	m.AddMigrations([]sqlutil.Migration{
		{
			Version: "roomserver: published appservice",
			Up:      deltas.UpPulishedAppservice,
		},
		{
			Version: "roomserver: published appservice pkey",
			Up:      deltas.UpPulishedAppservicePrimaryKey,
		},
	}...)
	return m.Up(context.Background())
}

func PreparePublishedTable(db *sql.DB) (tables.Published, error) {
	s := &publishedStatements{}

	return s, sqlutil.StatementList{
		{&s.upsertPublishedStmt, upsertPublishedSQL},
		{&s.selectAllPublishedStmt, selectAllPublishedSQL},
		{&s.selectPublishedStmt, selectPublishedSQL},
		{&s.selectNetworkPublishedStmt, selectNetworkPublishedSQL},
	}.Prepare(db)
}

func (s *publishedStatements) UpsertRoomPublished(
	ctx context.Context, txn *sql.Tx, roomID, appserviceID, networkID string, published bool,
) (err error) {
	stmt := sqlutil.TxStmt(txn, s.upsertPublishedStmt)
	_, err = stmt.ExecContext(ctx, roomID, appserviceID, networkID, published)
	return
}

func (s *publishedStatements) SelectPublishedFromRoomID(
	ctx context.Context, txn *sql.Tx, roomID string,
) (published bool, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectPublishedStmt)
	err = stmt.QueryRowContext(ctx, roomID).Scan(&published)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return
}

func (s *publishedStatements) SelectAllPublishedRooms(
	ctx context.Context, txn *sql.Tx, networkID string, published, includeAllNetworks bool,
) ([]string, error) {
	var rows *sql.Rows
	var err error
	if networkID != "" {
		stmt := sqlutil.TxStmt(txn, s.selectNetworkPublishedStmt)
		rows, err = stmt.QueryContext(ctx, published, networkID)
	} else {
		stmt := sqlutil.TxStmt(txn, s.selectAllPublishedStmt)
		rows, err = stmt.QueryContext(ctx, published, includeAllNetworks)

	}
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectAllPublishedStmt: rows.close() failed")

	var roomIDs []string
	var roomID string
	for rows.Next() {
		if err = rows.Scan(&roomID); err != nil {
			return nil, err
		}

		roomIDs = append(roomIDs, roomID)
	}
	return roomIDs, rows.Err()
}
