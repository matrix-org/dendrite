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
	"context"
	"database/sql"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const eventTypesSchema = `
-- Numeric versions of the event "type"s. Event types tend to be taken from a
-- small common pool. Assigning each a numeric ID should reduce the amount of
-- data that needs to be stored and fetched from the database.
-- It also means that many operations can work with int64 arrays rather than
-- string arrays which may help reduce GC pressure.
-- Well known event types are pre-assigned numeric IDs:
--   1 -> m.room.create
--   2 -> m.room.power_levels
--   3 -> m.room.join_rules
--   4 -> m.room.third_party_invite
--   5 -> m.room.member
--   6 -> m.room.redaction
--   7 -> m.room.history_visibility
-- Picking well-known numeric IDs for the events types that require special
-- attention during state conflict resolution means that we write that code
-- using numeric constants.
-- It also means that the numeric IDs for common event types should be
-- consistent between different instances which might make ad-hoc debugging
-- easier.
-- Other event types are automatically assigned numeric IDs starting from 2**16.
-- This leaves room to add more pre-assigned numeric IDs and clearly separates
-- the automatically assigned IDs from the pre-assigned IDs.
CREATE SEQUENCE IF NOT EXISTS roomserver_event_type_nid_seq START 65536;
CREATE TABLE IF NOT EXISTS roomserver_event_types (
    -- Local numeric ID for the event type.
    event_type_nid BIGINT PRIMARY KEY DEFAULT nextval('roomserver_event_type_nid_seq'),
    -- The string event_type.
    event_type TEXT NOT NULL CONSTRAINT roomserver_event_type_unique UNIQUE
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
const insertEventTypeNIDSQL = "" +
	"INSERT INTO roomserver_event_types (event_type) VALUES ($1)" +
	" ON CONFLICT ON CONSTRAINT roomserver_event_type_unique" +
	" DO NOTHING RETURNING (event_type_nid)"

const selectEventTypeNIDSQL = "" +
	"SELECT event_type_nid FROM roomserver_event_types WHERE event_type = $1"

// Bulk lookup from string event type to numeric ID for that event type.
// Takes an array of strings as the query parameter.
const bulkSelectEventTypeNIDSQL = "" +
	"SELECT event_type, event_type_nid FROM roomserver_event_types" +
	" WHERE event_type = ANY($1)"

type eventTypeStatements struct {
	insertEventTypeNIDStmt     *sql.Stmt
	selectEventTypeNIDStmt     *sql.Stmt
	bulkSelectEventTypeNIDStmt *sql.Stmt
}

func (s *eventTypeStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(eventTypesSchema)
	if err != nil {
		return
	}

	return statementList{
		{&s.insertEventTypeNIDStmt, insertEventTypeNIDSQL},
		{&s.selectEventTypeNIDStmt, selectEventTypeNIDSQL},
		{&s.bulkSelectEventTypeNIDStmt, bulkSelectEventTypeNIDSQL},
	}.prepare(db)
}

func (s *eventTypeStatements) insertEventTypeNID(
	ctx context.Context, eventType string,
) (types.EventTypeNID, error) {
	var eventTypeNID int64
	err := s.insertEventTypeNIDStmt.QueryRowContext(ctx, eventType).Scan(&eventTypeNID)
	return types.EventTypeNID(eventTypeNID), err
}

func (s *eventTypeStatements) selectEventTypeNID(
	ctx context.Context, eventType string,
) (types.EventTypeNID, error) {
	var eventTypeNID int64
	err := s.selectEventTypeNIDStmt.QueryRowContext(ctx, eventType).Scan(&eventTypeNID)
	return types.EventTypeNID(eventTypeNID), err
}

func (s *eventTypeStatements) bulkSelectEventTypeNID(
	ctx context.Context, eventTypes []string,
) (map[string]types.EventTypeNID, error) {
	rows, err := s.bulkSelectEventTypeNIDStmt.QueryContext(ctx, pq.StringArray(eventTypes))
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
