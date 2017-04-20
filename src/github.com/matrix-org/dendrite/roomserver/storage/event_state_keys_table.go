// Copyright 2017 Vector Creations Ltd
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

package storage

import (
	"database/sql"
	"github.com/lib/pq"
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
CREATE SEQUENCE IF NOT EXISTS event_state_key_nid_seq START 65536;
CREATE TABLE IF NOT EXISTS event_state_keys (
    -- Local numeric ID for the state key.
    event_state_key_nid BIGINT PRIMARY KEY DEFAULT nextval('event_state_key_nid_seq'),
    event_state_key TEXT NOT NULL CONSTRAINT event_state_key_unique UNIQUE
);
INSERT INTO event_state_keys (event_state_key_nid, event_state_key) VALUES
    (1, '') ON CONFLICT DO NOTHING;
`

// Same as insertEventTypeNIDSQL
const insertEventStateKeyNIDSQL = "" +
	"INSERT INTO event_state_keys (event_state_key) VALUES ($1)" +
	" ON CONFLICT ON CONSTRAINT event_state_key_unique" +
	" DO NOTHING RETURNING (event_state_key_nid)"

const selectEventStateKeyNIDSQL = "" +
	"SELECT event_state_key_nid FROM event_state_keys WHERE event_state_key = $1"

// Bulk lookup from string state key to numeric ID for that state key.
// Takes an array of strings as the query parameter.
const bulkSelectEventStateKeyNIDSQL = "" +
	"SELECT event_state_key, event_state_key_nid FROM event_state_keys" +
	" WHERE event_state_key = ANY($1)"

type eventStateKeyStatements struct {
	insertEventStateKeyNIDStmt     *sql.Stmt
	selectEventStateKeyNIDStmt     *sql.Stmt
	bulkSelectEventStateKeyNIDStmt *sql.Stmt
}

func (s *eventStateKeyStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(eventStateKeysSchema)
	if err != nil {
		return
	}
	return statementList{
		{&s.insertEventStateKeyNIDStmt, insertEventStateKeyNIDSQL},
		{&s.selectEventStateKeyNIDStmt, selectEventStateKeyNIDSQL},
		{&s.bulkSelectEventStateKeyNIDStmt, bulkSelectEventStateKeyNIDSQL},
	}.prepare(db)
}

func (s *eventStateKeyStatements) insertEventStateKeyNID(eventStateKey string) (types.EventStateKeyNID, error) {
	var eventStateKeyNID int64
	err := s.insertEventStateKeyNIDStmt.QueryRow(eventStateKey).Scan(&eventStateKeyNID)
	return types.EventStateKeyNID(eventStateKeyNID), err
}

func (s *eventStateKeyStatements) selectEventStateKeyNID(eventStateKey string) (types.EventStateKeyNID, error) {
	var eventStateKeyNID int64
	err := s.selectEventStateKeyNIDStmt.QueryRow(eventStateKey).Scan(&eventStateKeyNID)
	return types.EventStateKeyNID(eventStateKeyNID), err
}

func (s *eventStateKeyStatements) bulkSelectEventStateKeyNID(eventStateKeys []string) (map[string]types.EventStateKeyNID, error) {
	rows, err := s.bulkSelectEventStateKeyNIDStmt.Query(pq.StringArray(eventStateKeys))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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
