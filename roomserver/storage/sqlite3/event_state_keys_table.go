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

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
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

const insertEventStateKeyNIDResultSQL = `
	SELECT event_state_key_nid FROM roomserver_event_state_keys
		WHERE rowid = last_insert_rowid();
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
	insertEventStateKeyNIDStmt       *sql.Stmt
	insertEventStateKeyNIDResultStmt *sql.Stmt
	selectEventStateKeyNIDStmt       *sql.Stmt
	bulkSelectEventStateKeyNIDStmt   *sql.Stmt
	bulkSelectEventStateKeyStmt      *sql.Stmt
}

func (s *eventStateKeyStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(eventStateKeysSchema)
	if err != nil {
		return
	}
	return statementList{
		{&s.insertEventStateKeyNIDStmt, insertEventStateKeyNIDSQL},
		{&s.insertEventStateKeyNIDResultStmt, insertEventStateKeyNIDResultSQL},
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
	insertStmt := common.TxStmt(txn, s.insertEventStateKeyNIDStmt)
	selectStmt := common.TxStmt(txn, s.insertEventStateKeyNIDResultStmt)
	if _, err = insertStmt.ExecContext(ctx, eventStateKey); err == nil {
		err = selectStmt.QueryRowContext(ctx).Scan(&eventStateKeyNID)
		if err != nil {
			fmt.Println("insertEventStateKeyNID selectStmt.QueryRowContext:", err)
		}
	} else {
		fmt.Println("insertEventStateKeyNID insertStmt.ExecContext:", err)
	}
	return types.EventStateKeyNID(eventStateKeyNID), err
}

func (s *eventStateKeyStatements) selectEventStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKey string,
) (types.EventStateKeyNID, error) {
	var eventStateKeyNID int64
	stmt := common.TxStmt(txn, s.selectEventStateKeyNIDStmt)
	err := stmt.QueryRowContext(ctx, eventStateKey).Scan(&eventStateKeyNID)
	if err != nil {
		fmt.Println("selectEventStateKeyNID stmt.QueryRowContext:", err)
	}
	return types.EventStateKeyNID(eventStateKeyNID), err
}

func (s *eventStateKeyStatements) bulkSelectEventStateKeyNID(
	ctx context.Context, eventStateKeys []string,
) (map[string]types.EventStateKeyNID, error) {
	rows, err := s.bulkSelectEventStateKeyNIDStmt.QueryContext(
		ctx, pq.StringArray(eventStateKeys),
	)
	if err != nil {
		fmt.Println("bulkSelectEventStateKeyNID s.bulkSelectEventStateKeyNIDStmt.QueryContext:", err)
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	result := make(map[string]types.EventStateKeyNID, len(eventStateKeys))
	for rows.Next() {
		var stateKey string
		var stateKeyNID int64
		if err := rows.Scan(&stateKey, &stateKeyNID); err != nil {
			fmt.Println("bulkSelectEventStateKeyNID rows.Scan:", err)
			return nil, err
		}
		result[stateKey] = types.EventStateKeyNID(stateKeyNID)
	}
	return result, nil
}

func (s *eventStateKeyStatements) bulkSelectEventStateKey(
	ctx context.Context, eventStateKeyNIDs []types.EventStateKeyNID,
) (map[types.EventStateKeyNID]string, error) {
	nIDs := make(pq.Int64Array, len(eventStateKeyNIDs))
	for i := range eventStateKeyNIDs {
		nIDs[i] = int64(eventStateKeyNIDs[i])
	}
	rows, err := s.bulkSelectEventStateKeyStmt.QueryContext(ctx, nIDs)
	if err != nil {
		fmt.Println("bulkSelectEventStateKey s.bulkSelectEventStateKeyStmt.QueryContext:", err)
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	result := make(map[types.EventStateKeyNID]string, len(eventStateKeyNIDs))
	for rows.Next() {
		var stateKey string
		var stateKeyNID int64
		if err := rows.Scan(&stateKey, &stateKeyNID); err != nil {
			fmt.Println("bulkSelectEventStateKey rows.Scan:", err)
			return nil, err
		}
		result[types.EventStateKeyNID(stateKeyNID)] = stateKey
	}
	return result, nil
}
