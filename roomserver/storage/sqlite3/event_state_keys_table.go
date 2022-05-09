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
	"fmt"
	"strings"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const eventStateKeysSchema = `
	CREATE TABLE IF NOT EXISTS roomserver_event_state_keys (
    event_state_key_nid INTEGER PRIMARY KEY AUTOINCREMENT,
    event_state_key TEXT NOT NULL UNIQUE
	);
	INSERT INTO roomserver_event_state_keys (event_state_key_nid, event_state_key)
		VALUES (1, '')
		ON CONFLICT DO NOTHING;
`

// Same as insertEventTypeNIDSQL
const insertEventStateKeyNIDSQL = `
	INSERT INTO roomserver_event_state_keys (event_state_key) VALUES ($1)
	  ON CONFLICT DO NOTHING
	  RETURNING event_state_key_nid;
`

const selectEventStateKeyNIDSQL = `
	SELECT event_state_key_nid FROM roomserver_event_state_keys
	  WHERE event_state_key = $1
`

// Bulk lookup from string state key to numeric ID for that state key.
// Takes an array of strings as the query parameter.
const bulkSelectEventStateKeySQL = `
	SELECT event_state_key, event_state_key_nid FROM roomserver_event_state_keys
	  WHERE event_state_key IN ($1)
`

// Bulk lookup from numeric ID to string state key for that state key.
// Takes an array of strings as the query parameter.
const bulkSelectEventStateKeyNIDSQL = `
	SELECT event_state_key, event_state_key_nid FROM roomserver_event_state_keys
	  WHERE event_state_key_nid IN ($1)
`

type eventStateKeyStatements struct {
	db                             *sql.DB
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
	s := &eventStateKeyStatements{
		db: db,
	}

	return s, sqlutil.StatementList{
		{&s.insertEventStateKeyNIDStmt, insertEventStateKeyNIDSQL},
		{&s.selectEventStateKeyNIDStmt, selectEventStateKeyNIDSQL},
		{&s.bulkSelectEventStateKeyNIDStmt, bulkSelectEventStateKeyNIDSQL},
		{&s.bulkSelectEventStateKeyStmt, bulkSelectEventStateKeySQL},
	}.Prepare(db)
}

func (s *eventStateKeyStatements) InsertEventStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKey string,
) (eventStateKeyNID types.EventStateKeyNID, err error) {
	insertStmt := sqlutil.TxStmt(txn, s.insertEventStateKeyNIDStmt)
	if err := insertStmt.QueryRowContext(ctx, eventStateKey).Scan(&eventStateKeyNID); err != nil {
		return 0, fmt.Errorf("resultStmt.QueryRowContext.Scan: %w", err)
	}
	return
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
	iEventStateKeys := make([]interface{}, len(eventStateKeys))
	for k, v := range eventStateKeys {
		iEventStateKeys[k] = v
	}
	selectOrig := strings.Replace(bulkSelectEventStateKeySQL, "($1)", sqlutil.QueryVariadic(len(eventStateKeys)), 1)
	var rows *sql.Rows
	var err error
	if txn != nil {
		rows, err = txn.QueryContext(ctx, selectOrig, iEventStateKeys...)
	} else {
		rows, err = s.db.QueryContext(ctx, selectOrig, iEventStateKeys...)
	}
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
	return result, nil
}

func (s *eventStateKeyStatements) BulkSelectEventStateKey(
	ctx context.Context, txn *sql.Tx, eventStateKeyNIDs []types.EventStateKeyNID,
) (map[types.EventStateKeyNID]string, error) {
	iEventStateKeyNIDs := make([]interface{}, len(eventStateKeyNIDs))
	for k, v := range eventStateKeyNIDs {
		iEventStateKeyNIDs[k] = v
	}
	selectOrig := strings.Replace(bulkSelectEventStateKeyNIDSQL, "($1)", sqlutil.QueryVariadic(len(eventStateKeyNIDs)), 1)
	selectPrep, err := s.db.Prepare(selectOrig)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, selectPrep, "selectPrep.close() failed")
	stmt := sqlutil.TxStmt(txn, selectPrep)
	rows, err := stmt.QueryContext(ctx, iEventStateKeyNIDs...)
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
	return result, nil
}
