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

package mysql

import (
	"context"
	"database/sql"
	"strings"

	"github.com/matrix-org/dendrite/internal"

	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const eventTypesSchema = `
-- Numeric versions of the event "type"s. Event types tend to be taken from a
-- small internal pool. Assigning each a numeric ID should reduce the amount of
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
-- It also means that the numeric IDs for internal event types should be
-- consistent between different instances which might make ad-hoc debugging
-- easier.
-- Other event types are automatically assigned numeric IDs starting from 2**16.
-- This leaves room to add more pre-assigned numeric IDs and clearly separates
-- the automatically assigned IDs from the pre-assigned IDs.
CREATE TABLE IF NOT EXISTS roomserver_event_types (
    -- Local numeric ID for the event type.
    event_type_nid BIGINT AUTO_INCREMENT PRIMARY KEY,
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
const insertEventTypeNIDSQL = "" +
	"INSERT INTO roomserver_event_types (event_type) VALUES ($1)" +
	" ON CONFLICT DO NOTHING"

const insertEventTypeNIDResultSQL = "" +
	"SELECT event_type_nid FROM roomserver_event_types" +
	" WHERE rowid = last_insert_rowid()"

const selectEventTypeNIDSQL = "" +
	"SELECT event_type_nid FROM roomserver_event_types WHERE event_type = $1"

// Bulk lookup from string event type to numeric ID for that event type.
// Takes an array of strings as the query parameter.
const bulkSelectEventTypeNIDSQL = "" +
	"SELECT event_type, event_type_nid FROM roomserver_event_types" +
	" WHERE event_type = ANY($1)"

type eventTypeStatements struct {
	db                           *sql.DB
	insertEventTypeNIDStmt       *sql.Stmt
	insertEventTypeNIDResultStmt *sql.Stmt
	selectEventTypeNIDStmt       *sql.Stmt
	bulkSelectEventTypeNIDStmt   *sql.Stmt
}

func NewMysqlEventTypesTable(db *sql.DB) (tables.EventTypes, error) {
	s := &eventTypeStatements{}
	s.db = db
	_, err := db.Exec(eventTypesSchema)
	if err != nil {
		return nil, err
	}

	return s, shared.StatementList{
		{&s.insertEventTypeNIDStmt, insertEventTypeNIDSQL},
		{&s.insertEventTypeNIDResultStmt, insertEventTypeNIDResultSQL},
		{&s.selectEventTypeNIDStmt, selectEventTypeNIDSQL},
		{&s.bulkSelectEventTypeNIDStmt, bulkSelectEventTypeNIDSQL},
	}.Prepare(db)
}

func (s *eventTypeStatements) InsertEventTypeNID(
	ctx context.Context, txn *sql.Tx, eventType string,
) (types.EventTypeNID, error) {
	var eventTypeNID int64
	var err error
	insertStmt := internal.TxStmt(txn, s.insertEventTypeNIDStmt)
	resultStmt := internal.TxStmt(txn, s.insertEventTypeNIDResultStmt)
	if _, err = insertStmt.ExecContext(ctx, eventType); err == nil {
		err = resultStmt.QueryRowContext(ctx).Scan(&eventTypeNID)
	}
	return types.EventTypeNID(eventTypeNID), err
}

func (s *eventTypeStatements) SelectEventTypeNID(
	ctx context.Context, txn *sql.Tx, eventType string,
) (types.EventTypeNID, error) {
	var eventTypeNID int64
	err := txn.Stmt(s.selectEventTypeNIDStmt).QueryRowContext(ctx, eventType).Scan(&eventTypeNID)
	return types.EventTypeNID(eventTypeNID), err
}

func (s *eventTypeStatements) BulkSelectEventTypeNID(
	ctx context.Context, eventTypes []string,
) (map[string]types.EventTypeNID, error) {
	///////////////
	iEventTypes := make([]interface{}, len(eventTypes))
	for k, v := range eventTypes {
		iEventTypes[k] = v
	}
	selectOrig := strings.Replace(bulkSelectEventTypeNIDSQL, "($1)", internal.QueryVariadic(len(iEventTypes)), 1)
	selectPrep, err := s.db.Prepare(selectOrig)
	if err != nil {
		return nil, err
	}
	///////////////

	rows, err := selectPrep.QueryContext(ctx, iEventTypes...)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectEventTypeNID: rows.close() failed")

	result := make(map[string]types.EventTypeNID, len(eventTypes))
	for rows.Next() {
		var eventType string
		var eventTypeNID int64
		if err := rows.Scan(&eventType, &eventTypeNID); err != nil {
			return nil, err
		}
		result[eventType] = types.EventTypeNID(eventTypeNID)
	}
	return result, rows.Err()
}
