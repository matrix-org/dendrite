// Copyright 2022 The Matrix.org Foundation C.I.C.
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
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"
)

const countEventTypesSQL = "" +
	"SELECT COUNT(*) FROM syncapi_output_room_events" +
	" WHERE type = $1 AND id > $2 AND sender like $3"

const countActiveRoomsSQL = "" +
	"SELECT COUNT(DISTINCT room_id) FROM syncapi_output_room_events" +
	" WHERE type = $1 AND id > $2"

const countTotalRoomsSQL = "" +
	"SELECT COUNT(DISTINCT room_id) FROM syncapi_output_room_events"

type statsStatements struct {
	serverName           gomatrixserverlib.ServerName
	countTypesStmt       *sql.Stmt
	countActiveRoomsStmt *sql.Stmt
	countTotalRoomsStmt  *sql.Stmt
}

func PrepareStats(db *sql.DB, serverName gomatrixserverlib.ServerName) (tables.Stats, error) {
	s := &statsStatements{
		serverName: serverName,
	}
	return s, sqlutil.StatementList{
		{&s.countTypesStmt, countEventTypesSQL},
		{&s.countActiveRoomsStmt, countActiveRoomsSQL},
		{&s.countTotalRoomsStmt, countTotalRoomsSQL},
	}.Prepare(db)
}

func (s *statsStatements) DailyE2EEMessages(ctx context.Context, txn *sql.Tx, prevID int64) (result int64, err error) {
	stmt := sqlutil.TxStmt(txn, s.countTypesStmt)
	err = stmt.QueryRowContext(ctx,
		"m.room.encrypted",
		prevID, "%",
	).Scan(&result)
	return
}

func (s *statsStatements) DailySentE2EEMessages(ctx context.Context, txn *sql.Tx, prevID int64) (result int64, err error) {
	stmt := sqlutil.TxStmt(txn, s.countTypesStmt)
	err = stmt.QueryRowContext(ctx,
		"m.room.encrypted",
		prevID,
		fmt.Sprintf("%%:%s", s.serverName),
	).Scan(&result)
	return
}

func (s *statsStatements) DailyMessages(ctx context.Context, txn *sql.Tx, prevID int64) (result int64, err error) {
	stmt := sqlutil.TxStmt(txn, s.countTypesStmt)
	err = stmt.QueryRowContext(ctx,
		"m.room.message",
		prevID,
		"%",
	).Scan(&result)
	return
}

func (s *statsStatements) DailySentMessages(ctx context.Context, txn *sql.Tx, prevID int64) (result int64, err error) {
	stmt := sqlutil.TxStmt(txn, s.countTypesStmt)
	err = stmt.QueryRowContext(ctx,
		"m.room.message",
		prevID,
		fmt.Sprintf("%%:%s", s.serverName),
	).Scan(&result)
	return
}

func (s *statsStatements) DailyActiveE2EERooms(ctx context.Context, txn *sql.Tx, prevID int64) (result int64, err error) {
	stmt := sqlutil.TxStmt(txn, s.countActiveRoomsStmt)
	err = stmt.QueryRowContext(ctx,
		"m.room.encrypted",
		prevID,
	).Scan(&result)
	return
}

func (s *statsStatements) DailyActiveRooms(ctx context.Context, txn *sql.Tx, prevID int64) (result int64, err error) {
	stmt := sqlutil.TxStmt(txn, s.countActiveRoomsStmt)
	err = stmt.QueryRowContext(ctx,
		"m.room.message",
		prevID,
	).Scan(&result)
	return
}

func (s *statsStatements) TotalRooms(ctx context.Context, txn *sql.Tx) (result int64, err error) {
	stmt := sqlutil.TxStmt(txn, s.countTotalRoomsStmt)
	err = stmt.QueryRowContext(ctx).Scan(&result)
	return
}