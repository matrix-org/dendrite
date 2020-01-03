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

package postgres

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/gomatrix"
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
	" DO UPDATE SET id = EXCLUDED.id" +
	" RETURNING id"

const selectAccountDataInRangeSQL = "" +
	"SELECT room_id, type FROM syncapi_account_data_type" +
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

func (s *accountDataStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(accountDataSchema)
	if err != nil {
		return
	}
	if s.insertAccountDataStmt, err = db.Prepare(insertAccountDataSQL); err != nil {
		return
	}
	if s.selectAccountDataInRangeStmt, err = db.Prepare(selectAccountDataInRangeSQL); err != nil {
		return
	}
	if s.selectMaxAccountDataIDStmt, err = db.Prepare(selectMaxAccountDataIDSQL); err != nil {
		return
	}
	return
}

func (s *accountDataStatements) insertAccountData(
	ctx context.Context,
	userID, roomID, dataType string,
) (pos int64, err error) {
	err = s.insertAccountDataStmt.QueryRowContext(ctx, userID, roomID, dataType).Scan(&pos)
	return
}

func (s *accountDataStatements) selectAccountDataInRange(
	ctx context.Context,
	userID string,
	oldPos, newPos int64,
	accountDataFilterPart *gomatrix.FilterPart,
) (data map[string][]string, err error) {
	data = make(map[string][]string)

	// If both positions are the same, it means that the data was saved after the
	// latest room event. In that case, we need to decrement the old position as
	// it would prevent the SQL request from returning anything.
	if oldPos == newPos {
		oldPos--
	}

	rows, err := s.selectAccountDataInRangeStmt.QueryContext(ctx, userID, oldPos, newPos,
		pq.StringArray(filterConvertTypeWildcardToSQL(accountDataFilterPart.Types)),
		pq.StringArray(filterConvertTypeWildcardToSQL(accountDataFilterPart.NotTypes)),
		accountDataFilterPart.Limit,
	)
	if err != nil {
		return
	}

	for rows.Next() {
		var dataType string
		var roomID string

		if err = rows.Scan(&roomID, &dataType); err != nil {
			return
		}

		if len(data[roomID]) > 0 {
			data[roomID] = append(data[roomID], dataType)
		} else {
			data[roomID] = []string{dataType}
		}
	}

	return
}

func (s *accountDataStatements) selectMaxAccountDataID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64
	stmt := common.TxStmt(txn, s.selectMaxAccountDataIDStmt)
	err = stmt.QueryRowContext(ctx).Scan(&nullableID)
	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}
