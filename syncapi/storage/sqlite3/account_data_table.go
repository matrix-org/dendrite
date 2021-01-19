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
	" SET id = EXCLUDED.id"

const selectAccountDataInRangeSQL = "" +
	"SELECT room_id, type FROM syncapi_account_data_type" +
	" WHERE user_id = $1 AND id > $2 AND id <= $3" +
	" ORDER BY id ASC"

const selectMaxAccountDataIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_account_data_type"

type accountDataStatements struct {
	db                           *sql.DB
	streamIDStatements           *streamIDStatements
	insertAccountDataStmt        *sql.Stmt
	selectMaxAccountDataIDStmt   *sql.Stmt
	selectAccountDataInRangeStmt *sql.Stmt
}

func NewSqliteAccountDataTable(db *sql.DB, streamID *streamIDStatements) (tables.AccountData, error) {
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
	_, err = sqlutil.TxStmt(txn, s.insertAccountDataStmt).ExecContext(ctx, pos, userID, roomID, dataType)
	return
}

func (s *accountDataStatements) SelectAccountDataInRange(
	ctx context.Context,
	userID string,
	r types.Range,
	accountDataFilterPart *gomatrixserverlib.EventFilter,
) (data map[string][]string, err error) {
	data = make(map[string][]string)

	rows, err := s.selectAccountDataInRangeStmt.QueryContext(ctx, userID, r.Low(), r.High())
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectAccountDataInRange: rows.close() failed")

	var entries int

	for rows.Next() {
		var dataType string
		var roomID string

		if err = rows.Scan(&roomID, &dataType); err != nil {
			return
		}

		// check if we should add this by looking at the filter.
		// It would be nice if we could do this in SQL-land, but the mix of variadic
		// and positional parameters makes the query annoyingly hard to do, it's easier
		// and clearer to do it in Go-land. If there are no filters for [not]types then
		// this gets skipped.
		for _, includeType := range accountDataFilterPart.Types {
			if includeType != dataType { // TODO: wildcard support
				continue
			}
		}
		for _, excludeType := range accountDataFilterPart.NotTypes {
			if excludeType == dataType { // TODO: wildcard support
				continue
			}
		}

		if len(data[roomID]) > 0 {
			data[roomID] = append(data[roomID], dataType)
		} else {
			data[roomID] = []string{dataType}
		}
		entries++
		if entries >= accountDataFilterPart.Limit {
			break
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
