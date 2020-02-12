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

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const eventTypesSchema = `
  CREATE TABLE IF NOT EXISTS roomserver_event_types (
    event_type_nid INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL UNIQUE
  );
  INSERT INTO roomserver_event_types (event_type_nid, event_type) VALUES
    (1, 'm.room.create'),
    (2, 'm.room.power_levels'),
    (3, 'm.room.join_rules'),
    (4, 'm.room.third_party_invite'),
    (5, 'm.room.member'),
    (6, 'm.room.redaction'),
    (7, 'm.room.history_visibility') ON CONFLICT DO NOTHING;
`

// Assign a new numeric event type ID.
// The usual case is that the event type is not in the database.
// In that case the ID will be assigned using the next value from the sequence.
// We use `RETURNING` to tell postgres to return the assigned ID.
// But it's possible that the type was added in a query that raced with us.
// This will result in a conflict on the event_type_unique constraint, in this
// case we do nothing. Postgresql won't return a row in that case so we rely on
// the caller catching the sql.ErrNoRows error and running a select to get the row.
// We could get postgresql to return the row on a conflict by updating the row
// but it doesn't seem like a good idea to modify the rows just to make postgresql
// return it. Modifying the rows will cause postgres to assign a new tuple for the
// row even though the data doesn't change resulting in unncesssary modifications
// to the indexes.
const insertEventTypeNIDSQL = `
	INSERT INTO roomserver_event_types (event_type) VALUES ($1)
	  ON CONFLICT DO NOTHING;
`

const insertEventTypeNIDResultSQL = `
	SELECT event_type_nid FROM roomserver_event_types
		WHERE rowid = last_insert_rowid();
`

const selectEventTypeNIDSQL = `
	SELECT event_type_nid FROM roomserver_event_types WHERE event_type = $1
`

// Bulk lookup from string event type to numeric ID for that event type.
// Takes an array of strings as the query parameter.
const bulkSelectEventTypeNIDSQL = `
	SELECT event_type, event_type_nid FROM roomserver_event_types
	  WHERE event_type IN ($1)
`

type eventTypeStatements struct {
	db                           *sql.DB
	insertEventTypeNIDStmt       *sql.Stmt
	insertEventTypeNIDResultStmt *sql.Stmt
	selectEventTypeNIDStmt       *sql.Stmt
	bulkSelectEventTypeNIDStmt   *sql.Stmt
}

func (s *eventTypeStatements) prepare(db *sql.DB) (err error) {
	s.db = db
	_, err = db.Exec(eventTypesSchema)
	if err != nil {
		return
	}

	return statementList{
		{&s.insertEventTypeNIDStmt, insertEventTypeNIDSQL},
		{&s.insertEventTypeNIDResultStmt, insertEventTypeNIDResultSQL},
		{&s.selectEventTypeNIDStmt, selectEventTypeNIDSQL},
		{&s.bulkSelectEventTypeNIDStmt, bulkSelectEventTypeNIDSQL},
	}.prepare(db)
}

func (s *eventTypeStatements) insertEventTypeNID(
	ctx context.Context, tx *sql.Tx, eventType string,
) (types.EventTypeNID, error) {
	var eventTypeNID int64
	var err error
	insertStmt := common.TxStmt(tx, s.insertEventTypeNIDStmt)
	resultStmt := common.TxStmt(tx, s.insertEventTypeNIDResultStmt)
	if _, err = insertStmt.ExecContext(ctx, eventType); err == nil {
		err = resultStmt.QueryRowContext(ctx).Scan(&eventTypeNID)
	}
	return types.EventTypeNID(eventTypeNID), err
}

func (s *eventTypeStatements) selectEventTypeNID(
	ctx context.Context, tx *sql.Tx, eventType string,
) (types.EventTypeNID, error) {
	var eventTypeNID int64
	selectStmt := common.TxStmt(tx, s.selectEventTypeNIDStmt)
	err := selectStmt.QueryRowContext(ctx, eventType).Scan(&eventTypeNID)
	return types.EventTypeNID(eventTypeNID), err
}

func (s *eventTypeStatements) bulkSelectEventTypeNID(
	ctx context.Context, tx *sql.Tx, eventTypes []string,
) (map[string]types.EventTypeNID, error) {
	///////////////
	iEventTypes := make([]interface{}, len(eventTypes))
	for k, v := range eventTypes {
		iEventTypes[k] = v
	}
	selectOrig := strings.Replace(bulkSelectEventTypeNIDSQL, "($1)", common.QueryVariadic(len(iEventTypes)), 1)
	selectPrep, err := s.db.Prepare(selectOrig)
	if err != nil {
		return nil, err
	}
	///////////////

	selectStmt := common.TxStmt(tx, selectPrep)
	rows, err := selectStmt.QueryContext(ctx, iEventTypes...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck

	result := make(map[string]types.EventTypeNID, len(eventTypes))
	for rows.Next() {
		var eventType string
		var eventTypeNID int64
		if err := rows.Scan(&eventType, &eventTypeNID); err != nil {
			return nil, err
		}
		result[eventType] = types.EventTypeNID(eventTypeNID)
	}
	return result, nil
}
