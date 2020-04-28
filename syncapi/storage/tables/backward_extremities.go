// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package tables

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/common"
)

// BackwardsExtremitiesStatements contains the SQL statements to implement.
// See BackwardsExtremities to see the parameter and response types.
type BackwardsExtremitiesStatements interface {
	Schema() string
	InsertBackwardExtremity() string
	SelectBackwardExtremitiesForRoom() string
	DeleteBackwardExtremity() string
}

type PostgresBackwardsExtremitiesStatements struct{}

func (s *PostgresBackwardsExtremitiesStatements) Schema() string {
	return `-- Stores output room events received from the roomserver.
	CREATE TABLE IF NOT EXISTS syncapi_backward_extremities (
		-- The 'room_id' key for the event.
		room_id TEXT NOT NULL,
		-- The event ID for the last known event. This is the backwards extremity.
		event_id TEXT NOT NULL,
		-- The prev_events for the last known event. This is used to update extremities.
		prev_event_id TEXT NOT NULL,
	
		PRIMARY KEY(room_id, event_id, prev_event_id)
	);
	`
}
func (s *PostgresBackwardsExtremitiesStatements) InsertBackwardExtremity() string {
	return "" +
		"INSERT INTO syncapi_backward_extremities (room_id, event_id, prev_event_id)" +
		" VALUES ($1, $2, $3)" +
		" ON CONFLICT DO NOTHING"
}
func (s *PostgresBackwardsExtremitiesStatements) SelectBackwardExtremitiesForRoom() string {
	return "SELECT DISTINCT event_id FROM syncapi_backward_extremities WHERE room_id = $1"
}
func (s *PostgresBackwardsExtremitiesStatements) DeleteBackwardExtremity() string {
	return "DELETE FROM syncapi_backward_extremities WHERE room_id = $1 AND prev_event_id = $2"
}

type SqliteBackwardsExtremitiesStatements struct{}

func (s *SqliteBackwardsExtremitiesStatements) Schema() string {
	return `-- Stores output room events received from the roomserver.
CREATE TABLE IF NOT EXISTS syncapi_backward_extremities (
	-- The 'room_id' key for the event.
	room_id TEXT NOT NULL,
	-- The event ID for the last known event. This is the backwards extremity.
	event_id TEXT NOT NULL,
	-- The prev_events for the last known event. This is used to update extremities.
	prev_event_id TEXT NOT NULL,

	PRIMARY KEY(room_id, event_id, prev_event_id)
);
`
}

func (s *SqliteBackwardsExtremitiesStatements) InsertBackwardExtremity() string {
	return "" +
		"INSERT INTO syncapi_backward_extremities (room_id, event_id, prev_event_id)" +
		" VALUES ($1, $2, $3)" +
		" ON CONFLICT (room_id, event_id, prev_event_id) DO NOTHING"
}

func (s *SqliteBackwardsExtremitiesStatements) SelectBackwardExtremitiesForRoom() string {
	return "" +
		"SELECT DISTINCT event_id FROM syncapi_backward_extremities WHERE room_id = $1"
}

func (s *SqliteBackwardsExtremitiesStatements) DeleteBackwardExtremity() string {
	return "" +
		"DELETE FROM syncapi_backward_extremities WHERE room_id = $1 AND prev_event_id = $2"
}

// BackwardsExtremities keeps track of backwards extremities for a room.
// Backwards extremities are the earliest (DAG-wise) known events which we have
// the entire event JSON. These event IDs are used in federation requests to fetch
// even earlier events.
//
// We persist the previous event IDs as well, one per row, so when we do fetch even
// earlier events we can simply delete rows which referenced it. Consider the graph:
//        A
//        |   Event C has 1 prev_event ID: A.
//    B   C
//    |___|   Event D has 2 prev_event IDs: B and C.
//      |
//      D
// The earliest known event we have is D, so this table has 2 rows.
// A backfill request gives us C but not B. We delete rows where prev_event=C. This
// still means that D is a backwards extremity as we do not have event B. However, event
// C is *also* a backwards extremity at this point as we do not have event A. Later,
// when we fetch event B, we delete rows where prev_event=B. This then removes D as
// a backwards extremity because there are no more rows with event_id=B.
type BackwardsExtremities struct {
	insertBackwardExtremityStmt          *sql.Stmt
	selectBackwardExtremitiesForRoomStmt *sql.Stmt
	deleteBackwardExtremityStmt          *sql.Stmt
}

// NewBackwardsExtremities prepares the table
func NewBackwardsExtremities(db *sql.DB, stmts BackwardsExtremitiesStatements) (table BackwardsExtremities, err error) {
	_, err = db.Exec(stmts.Schema())
	if err != nil {
		return
	}
	if table.insertBackwardExtremityStmt, err = db.Prepare(stmts.InsertBackwardExtremity()); err != nil {
		return
	}
	if table.selectBackwardExtremitiesForRoomStmt, err = db.Prepare(stmts.SelectBackwardExtremitiesForRoom()); err != nil {
		return
	}
	if table.deleteBackwardExtremityStmt, err = db.Prepare(stmts.DeleteBackwardExtremity()); err != nil {
		return
	}
	return
}

// InsertsBackwardExtremity inserts a new backwards extremity.
func (s *BackwardsExtremities) InsertsBackwardExtremity(
	ctx context.Context, txn *sql.Tx, roomID, eventID string, prevEventID string,
) (err error) {
	_, err = txn.Stmt(s.insertBackwardExtremityStmt).ExecContext(ctx, roomID, eventID, prevEventID)
	return
}

// SelectBackwardExtremitiesForRoom retrieves all backwards extremities for the room.
func (s *BackwardsExtremities) SelectBackwardExtremitiesForRoom(
	ctx context.Context, roomID string,
) (eventIDs []string, err error) {
	rows, err := s.selectBackwardExtremitiesForRoomStmt.QueryContext(ctx, roomID)
	if err != nil {
		return
	}
	defer common.CloseAndLogIfError(ctx, rows, "selectBackwardExtremitiesForRoom: rows.close() failed")

	for rows.Next() {
		var eID string
		if err = rows.Scan(&eID); err != nil {
			return
		}

		eventIDs = append(eventIDs, eID)
	}

	return eventIDs, rows.Err()
}

// DeleteBackwardExtremity removes a backwards extremity for a room, if one existed.
func (s *BackwardsExtremities) DeleteBackwardExtremity(
	ctx context.Context, txn *sql.Tx, roomID, knownEventID string,
) (err error) {
	_, err = txn.Stmt(s.deleteBackwardExtremityStmt).ExecContext(ctx, roomID, knownEventID)
	return
}
