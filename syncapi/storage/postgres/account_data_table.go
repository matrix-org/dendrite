// Copyright 2024 New Vector Ltd.
// Copyright 2019, 2020 The Matrix.org Foundation C.I.C.
// Copyright 2017, 2018 New Vector Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/syncapi/storage/tables"
	"github.com/element-hq/dendrite/syncapi/synctypes"
	"github.com/element-hq/dendrite/syncapi/types"
	"github.com/lib/pq"
)

const accountDataSchema = `
-- This sequence is shared between all the tables generated from kafka logs.
CREATE SEQUENCE IF NOT EXISTS syncapi_stream_id;

-- Stores the types of account data that a user set has globally and in each room
-- and the stream ID when that type was last updated.
CREATE TABLE IF NOT EXISTS syncapi_account_data_type (
    -- An incrementing ID which denotes the position in the log that this event resides at.
    id BIGINT PRIMARY KEY DEFAULT nextval('syncapi_stream_id'),
    -- ID of the user the data belongs to
    user_id TEXT NOT NULL,
    -- ID of the room the data is related to (empty string if not related to a specific room)
    room_id TEXT NOT NULL,
    -- Type of the data
    type TEXT NOT NULL,

    -- We don't want two entries of the same type for the same user
    CONSTRAINT syncapi_account_data_unique UNIQUE (user_id, room_id, type)
);

CREATE UNIQUE INDEX IF NOT EXISTS syncapi_account_data_id_idx ON syncapi_account_data_type(id, type);
`

const insertAccountDataSQL = "" +
	"INSERT INTO syncapi_account_data_type (user_id, room_id, type) VALUES ($1, $2, $3)" +
	" ON CONFLICT ON CONSTRAINT syncapi_account_data_unique" +
	" DO UPDATE SET id = nextval('syncapi_stream_id')" +
	" RETURNING id"

const selectAccountDataInRangeSQL = "" +
	"SELECT id, room_id, type FROM syncapi_account_data_type" +
	" WHERE user_id = $1 AND id > $2 AND id <= $3" +
	" AND ( $4::text[] IS NULL OR     type LIKE ANY($4)  )" +
	" AND ( $5::text[] IS NULL OR NOT(type LIKE ANY($5)) )" +
	" ORDER BY id ASC LIMIT $6"

const selectMaxAccountDataIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_account_data_type"

type accountDataStatements struct {
	insertAccountDataStmt        *sql.Stmt
	selectAccountDataInRangeStmt *sql.Stmt
	selectMaxAccountDataIDStmt   *sql.Stmt
}

func NewPostgresAccountDataTable(db *sql.DB) (tables.AccountData, error) {
	s := &accountDataStatements{}
	_, err := db.Exec(accountDataSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.insertAccountDataStmt, insertAccountDataSQL},
		{&s.selectAccountDataInRangeStmt, selectAccountDataInRangeSQL},
		{&s.selectMaxAccountDataIDStmt, selectMaxAccountDataIDSQL},
	}.Prepare(db)
}

func (s *accountDataStatements) InsertAccountData(
	ctx context.Context, txn *sql.Tx,
	userID, roomID, dataType string,
) (pos types.StreamPosition, err error) {
	err = s.insertAccountDataStmt.QueryRowContext(ctx, userID, roomID, dataType).Scan(&pos)
	return
}

func (s *accountDataStatements) SelectAccountDataInRange(
	ctx context.Context, txn *sql.Tx,
	userID string,
	r types.Range,
	accountDataEventFilter *synctypes.EventFilter,
) (data map[string][]string, pos types.StreamPosition, err error) {
	data = make(map[string][]string)

	rows, err := sqlutil.TxStmt(txn, s.selectAccountDataInRangeStmt).QueryContext(
		ctx, userID, r.Low(), r.High(),
		pq.StringArray(filterConvertTypeWildcardToSQL(accountDataEventFilter.Types)),
		pq.StringArray(filterConvertTypeWildcardToSQL(accountDataEventFilter.NotTypes)),
		accountDataEventFilter.Limit,
	)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectAccountDataInRange: rows.close() failed")

	var dataType string
	var roomID string
	var id types.StreamPosition

	for rows.Next() {
		if err = rows.Scan(&id, &roomID, &dataType); err != nil {
			return
		}

		if len(data[roomID]) > 0 {
			data[roomID] = append(data[roomID], dataType)
		} else {
			data[roomID] = []string{dataType}
		}
		if id > pos {
			pos = id
		}
	}
	if pos == 0 {
		pos = r.High()
	}
	return data, pos, rows.Err()
}

func (s *accountDataStatements) SelectMaxAccountDataID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64
	stmt := sqlutil.TxStmt(txn, s.selectMaxAccountDataIDStmt)
	err = stmt.QueryRowContext(ctx).Scan(&nullableID)
	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}
