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
	"strings"

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
	  ON CONFLICT DO NOTHING;
`

const selectEventStateKeyNIDSQL = `
	SELECT event_state_key_nid FROM roomserver_event_state_keys
	  WHERE event_state_key = $1
`

// Bulk lookup from string state key to numeric ID for that state key.
// Takes an array of strings as the query parameter.
const bulkSelectEventStateKeyNIDSQL = `
	SELECT event_state_key, event_state_key_nid FROM roomserver_event_state_keys
	  WHERE event_state_key IN ($1)
`

// Bulk lookup from numeric ID to string state key for that state key.
// Takes an array of strings as the query parameter.
const bulkSelectEventStateKeySQL = `
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

func (s *eventStateKeyStatements) prepare(db *sql.DB) (err error) {
	s.db = db
	_, err = db.Exec(eventStateKeysSchema)
	if err != nil {
		return
	}
	return statementList{
		{&s.insertEventStateKeyNIDStmt, insertEventStateKeyNIDSQL},
		{&s.selectEventStateKeyNIDStmt, selectEventStateKeyNIDSQL},
		{&s.bulkSelectEventStateKeyNIDStmt, bulkSelectEventStateKeyNIDSQL},
		{&s.bulkSelectEventStateKeyStmt, bulkSelectEventStateKeySQL},
	}.prepare(db)
}

func (s *eventStateKeyStatements) insertEventStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKey string,
) (types.EventStateKeyNID, error) {
	var eventStateKeyNID int64
	var err error
	var res sql.Result
	insertStmt := txn.Stmt(s.insertEventStateKeyNIDStmt)
	if res, err = insertStmt.ExecContext(ctx, eventStateKey); err == nil {
		eventStateKeyNID, err = res.LastInsertId()
	}
	return types.EventStateKeyNID(eventStateKeyNID), err
}

func (s *eventStateKeyStatements) selectEventStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKey string,
) (types.EventStateKeyNID, error) {
	var eventStateKeyNID int64
	stmt := txn.Stmt(s.selectEventStateKeyNIDStmt)
	err := stmt.QueryRowContext(ctx, eventStateKey).Scan(&eventStateKeyNID)
	return types.EventStateKeyNID(eventStateKeyNID), err
}

func (s *eventStateKeyStatements) bulkSelectEventStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKeys []string,
) (map[string]types.EventStateKeyNID, error) {
	iEventStateKeys := make([]interface{}, len(eventStateKeys))
	for k, v := range eventStateKeys {
		iEventStateKeys[k] = v
	}
	selectOrig := strings.Replace(bulkSelectEventStateKeyNIDSQL, "($1)", queryVariadic(len(eventStateKeys)), 1)

	rows, err := txn.QueryContext(ctx, selectOrig, iEventStateKeys...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	result := make(map[string]types.EventStateKeyNID, len(eventStateKeys))
	for rows.Next() {
		var stateKey string
		var stateKeyNID int64
		if err := rows.Scan(&stateKey, &stateKeyNID); err != nil {
			return nil, err
		}
		result[stateKey] = types.EventStateKeyNID(stateKeyNID)
	}
	return result, nil
}

func (s *eventStateKeyStatements) bulkSelectEventStateKey(
	ctx context.Context, txn *sql.Tx, eventStateKeyNIDs []types.EventStateKeyNID,
) (map[types.EventStateKeyNID]string, error) {
	iEventStateKeyNIDs := make([]interface{}, len(eventStateKeyNIDs))
	for k, v := range eventStateKeyNIDs {
		iEventStateKeyNIDs[k] = v
	}
	selectOrig := strings.Replace(bulkSelectEventStateKeyNIDSQL, "($1)", queryVariadic(len(eventStateKeyNIDs)), 1)

	rows, err := txn.QueryContext(ctx, selectOrig, iEventStateKeyNIDs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	result := make(map[types.EventStateKeyNID]string, len(eventStateKeyNIDs))
	for rows.Next() {
		var stateKey string
		var stateKeyNID int64
		if err := rows.Scan(&stateKey, &stateKeyNID); err != nil {
			return nil, err
		}
		result[types.EventStateKeyNID(stateKeyNID)] = stateKey
	}
	return result, nil
}
