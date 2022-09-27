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
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const eventStateKeysSchema = `
-- Numeric versions of the event "state_key"s. State keys tend to be reused so
-- assigning each string a numeric ID should reduce the amount of data that
-- needs to be stored and fetched from the database.
-- It also means that many operations can work with int64 arrays rather than
-- string arrays which may help reduce GC pressure.
-- Well known state keys are pre-assigned numeric IDs:
--   1 -> "" (the empty string)
-- Other state keys are automatically assigned numeric IDs starting from 2**16.
-- This leaves room to add more pre-assigned numeric IDs and clearly separates
-- the automatically assigned IDs from the pre-assigned IDs.
CREATE SEQUENCE IF NOT EXISTS roomserver_event_state_key_nid_seq START 65536;
CREATE TABLE IF NOT EXISTS roomserver_event_state_keys (
    -- Local numeric ID for the state key.
    event_state_key_nid BIGINT PRIMARY KEY DEFAULT nextval('roomserver_event_state_key_nid_seq'),
    event_state_key TEXT NOT NULL CONSTRAINT roomserver_event_state_key_unique UNIQUE
);
INSERT INTO roomserver_event_state_keys (event_state_key_nid, event_state_key) VALUES
    (1, '') ON CONFLICT DO NOTHING;
`

// Same as insertEventTypeNIDSQL
const insertEventStateKeyNIDSQL = "" +
	"INSERT INTO roomserver_event_state_keys (event_state_key) VALUES ($1)" +
	" ON CONFLICT ON CONSTRAINT roomserver_event_state_key_unique" +
	" DO NOTHING RETURNING (event_state_key_nid)"

const selectEventStateKeyNIDSQL = "" +
	"SELECT event_state_key_nid FROM roomserver_event_state_keys" +
	" WHERE event_state_key = $1"

// Bulk lookup from string state key to numeric ID for that state key.
// Takes an array of strings as the query parameter.
const bulkSelectEventStateKeyNIDSQL = "" +
	"SELECT event_state_key, event_state_key_nid FROM roomserver_event_state_keys" +
	" WHERE event_state_key = ANY($1)"

// Bulk lookup from numeric ID to string state key for that state key.
// Takes an array of strings as the query parameter.
const bulkSelectEventStateKeySQL = "" +
	"SELECT event_state_key, event_state_key_nid FROM roomserver_event_state_keys" +
	" WHERE event_state_key_nid = ANY($1)"

type eventStateKeyStatements struct {
	insertEventStateKeyNIDStmt     *sql.Stmt
	selectEventStateKeyNIDStmt     *sql.Stmt
	bulkSelectEventStateKeyNIDStmt *sql.Stmt
	bulkSelectEventStateKeyStmt    *sql.Stmt
}

func CreateEventStateKeysTable(db *sql.DB) error {
	_, err := db.Exec(eventStateKeysSchema)
	return err
}

func PrepareEventStateKeysTable(db *sql.DB) (tables.EventStateKeys, error) {
	s := &eventStateKeyStatements{}

	return s, sqlutil.StatementList{
		{&s.insertEventStateKeyNIDStmt, insertEventStateKeyNIDSQL},
		{&s.selectEventStateKeyNIDStmt, selectEventStateKeyNIDSQL},
		{&s.bulkSelectEventStateKeyNIDStmt, bulkSelectEventStateKeyNIDSQL},
		{&s.bulkSelectEventStateKeyStmt, bulkSelectEventStateKeySQL},
	}.Prepare(db)
}

func (s *eventStateKeyStatements) InsertEventStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKey string,
) (types.EventStateKeyNID, error) {
	var eventStateKeyNID int64
	stmt := sqlutil.TxStmt(txn, s.insertEventStateKeyNIDStmt)
	err := stmt.QueryRowContext(ctx, eventStateKey).Scan(&eventStateKeyNID)
	return types.EventStateKeyNID(eventStateKeyNID), err
}

func (s *eventStateKeyStatements) SelectEventStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKey string,
) (types.EventStateKeyNID, error) {
	var eventStateKeyNID int64
	stmt := sqlutil.TxStmt(txn, s.selectEventStateKeyNIDStmt)
	err := stmt.QueryRowContext(ctx, eventStateKey).Scan(&eventStateKeyNID)
	return types.EventStateKeyNID(eventStateKeyNID), err
}

func (s *eventStateKeyStatements) BulkSelectEventStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKeys []string,
) (map[string]types.EventStateKeyNID, error) {
	stmt := sqlutil.TxStmt(txn, s.bulkSelectEventStateKeyNIDStmt)
	rows, err := stmt.QueryContext(
		ctx, pq.StringArray(eventStateKeys),
	)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectEventStateKeyNID: rows.close() failed")

	result := make(map[string]types.EventStateKeyNID, len(eventStateKeys))
	var stateKey string
	var stateKeyNID int64
	for rows.Next() {
		if err := rows.Scan(&stateKey, &stateKeyNID); err != nil {
			return nil, err
		}
		result[stateKey] = types.EventStateKeyNID(stateKeyNID)
	}
	return result, rows.Err()
}

func (s *eventStateKeyStatements) BulkSelectEventStateKey(
	ctx context.Context, txn *sql.Tx, eventStateKeyNIDs []types.EventStateKeyNID,
) (map[types.EventStateKeyNID]string, error) {
	nIDs := make(pq.Int64Array, len(eventStateKeyNIDs))
	for i := range eventStateKeyNIDs {
		nIDs[i] = int64(eventStateKeyNIDs[i])
	}
	stmt := sqlutil.TxStmt(txn, s.bulkSelectEventStateKeyStmt)
	rows, err := stmt.QueryContext(ctx, nIDs)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectEventStateKey: rows.close() failed")

	result := make(map[types.EventStateKeyNID]string, len(eventStateKeyNIDs))
	var stateKey string
	var stateKeyNID int64
	for rows.Next() {
		if err := rows.Scan(&stateKey, &stateKeyNID); err != nil {
			return nil, err
		}
		result[types.EventStateKeyNID(stateKeyNID)] = stateKey
	}
	return result, rows.Err()
}
