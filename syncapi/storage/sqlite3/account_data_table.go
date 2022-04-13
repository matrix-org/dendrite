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

package sqlite3

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

const accountDataSchema = `
CREATE TABLE IF NOT EXISTS syncapi_account_data_type (
    id INTEGER PRIMARY KEY,
    user_id TEXT NOT NULL,
    room_id TEXT NOT NULL,
    type TEXT NOT NULL,
    UNIQUE (user_id, room_id, type)
);
`

const insertAccountDataSQL = "" +
	"INSERT INTO syncapi_account_data_type (id, user_id, room_id, type) VALUES ($1, $2, $3, $4)" +
	" ON CONFLICT (user_id, room_id, type) DO UPDATE" +
	" SET id = $5"

// further parameters are added by prepareWithFilters
const selectAccountDataInRangeSQL = "" +
	"SELECT room_id, type FROM syncapi_account_data_type" +
	" WHERE user_id = $1 AND id > $2 AND id <= $3"

const selectMaxAccountDataIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_account_data_type"

type accountDataStatements struct {
	db                           *sql.DB
	streamIDStatements           *StreamIDStatements
	insertAccountDataStmt        *sql.Stmt
	selectMaxAccountDataIDStmt   *sql.Stmt
	selectAccountDataInRangeStmt *sql.Stmt
}

func NewSqliteAccountDataTable(db *sql.DB, streamID *StreamIDStatements) (tables.AccountData, error) {
	s := &accountDataStatements{
		db:                 db,
		streamIDStatements: streamID,
	}
	_, err := db.Exec(accountDataSchema)
	if err != nil {
		return nil, err
	}
	if s.insertAccountDataStmt, err = db.Prepare(insertAccountDataSQL); err != nil {
		return nil, err
	}
	if s.selectMaxAccountDataIDStmt, err = db.Prepare(selectMaxAccountDataIDSQL); err != nil {
		return nil, err
	}
	if s.selectAccountDataInRangeStmt, err = db.Prepare(selectAccountDataInRangeSQL); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *accountDataStatements) InsertAccountData(
	ctx context.Context, txn *sql.Tx,
	userID, roomID, dataType string,
) (pos types.StreamPosition, err error) {
	pos, err = s.streamIDStatements.nextAccountDataID(ctx, txn)
	if err != nil {
		return
	}
	_, err = sqlutil.TxStmt(txn, s.insertAccountDataStmt).ExecContext(ctx, pos, userID, roomID, dataType, pos)
	return
}

func (s *accountDataStatements) SelectAccountDataInRange(
	ctx context.Context,
	userID string,
	r types.Range,
	filter *gomatrixserverlib.EventFilter,
) (data map[string][]string, err error) {
	data = make(map[string][]string)
	stmt, params, err := prepareWithFilters(
		s.db, nil, selectAccountDataInRangeSQL,
		[]interface{}{
			userID, r.Low(), r.High(),
		},
		filter.Senders, filter.NotSenders,
		filter.Types, filter.NotTypes,
		[]string{}, nil, filter.Limit, FilterOrderAsc)

	rows, err := stmt.QueryContext(ctx, params...)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectAccountDataInRange: rows.close() failed")

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

	return data, nil
}

func (s *accountDataStatements) SelectMaxAccountDataID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64
	err = sqlutil.TxStmt(txn, s.selectMaxAccountDataIDStmt).QueryRowContext(ctx).Scan(&nullableID)
	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}
