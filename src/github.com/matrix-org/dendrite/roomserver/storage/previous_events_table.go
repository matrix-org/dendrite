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

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const previousEventSchema = `
-- The previous events table stores the event_ids referenced by the events
-- stored in the events table.
-- This is used to tell if a new event is already referenced by an event in
-- the database.
CREATE TABLE IF NOT EXISTS roomserver_previous_events (
    -- The string event ID taken from the prev_events key of an event.
    previous_event_id TEXT NOT NULL,
    -- The SHA256 reference hash taken from the prev_events key of an event.
    previous_reference_sha256 BYTEA NOT NULL,
    -- A list of numeric event IDs of events that reference this prev_event.
    event_nids BIGINT[] NOT NULL,
    CONSTRAINT roomserver_previous_event_id_unique UNIQUE (previous_event_id, previous_reference_sha256)
);
`

// Insert an entry into the previous_events table.
// If there is already an entry indicating that an event references that previous event then
// add the event NID to the list to indicate that this event references that previous event as well.
// This should only be modified while holding a "FOR UPDATE" lock on the row in the rooms table for this room.
// The lock is necessary to avoid data races when checking whether an event is already referenced by another event.
const insertPreviousEventSQL = "" +
	"INSERT INTO roomserver_previous_events" +
	" (previous_event_id, previous_reference_sha256, event_nids)" +
	" VALUES ($1, $2, array_append('{}'::bigint[], $3))" +
	" ON CONFLICT ON CONSTRAINT roomserver_previous_event_id_unique" +
	" DO UPDATE SET event_nids = array_append(roomserver_previous_events.event_nids, $3)" +
	" WHERE $3 != ALL(roomserver_previous_events.event_nids)"

// Check if the event is referenced by another event in the table.
// This should only be done while holding a "FOR UPDATE" lock on the row in the rooms table for this room.
const selectPreviousEventExistsSQL = "" +
	"SELECT 1 FROM roomserver_previous_events" +
	" WHERE previous_event_id = $1 AND previous_reference_sha256 = $2"

type previousEventStatements struct {
	insertPreviousEventStmt       *sql.Stmt
	selectPreviousEventExistsStmt *sql.Stmt
}

func (s *previousEventStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(previousEventSchema)
	if err != nil {
		return
	}

	return statementList{
		{&s.insertPreviousEventStmt, insertPreviousEventSQL},
		{&s.selectPreviousEventExistsStmt, selectPreviousEventExistsSQL},
	}.prepare(db)
}

func (s *previousEventStatements) insertPreviousEvent(txn *sql.Tx, previousEventID string, previousEventReferenceSHA256 []byte, eventNID types.EventNID) error {
	_, err := common.TxStmt(txn, s.insertPreviousEventStmt).Exec(previousEventID, previousEventReferenceSHA256, int64(eventNID))
	return err
}

// Check if the event reference exists
// Returns sql.ErrNoRows if the event reference doesn't exist.
func (s *previousEventStatements) selectPreviousEventExists(txn *sql.Tx, eventID string, eventReferenceSHA256 []byte) error {
	var ok int64
	return common.TxStmt(txn, s.selectPreviousEventExistsStmt).QueryRow(eventID, eventReferenceSHA256).Scan(&ok)
}
