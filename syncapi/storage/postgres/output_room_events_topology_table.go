// Copyright 2018 New Vector Ltd
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

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

const outputRoomEventsTopologySchema = `
-- Stores output room events received from the roomserver.
CREATE TABLE IF NOT EXISTS syncapi_output_room_events_topology (
	-- The event ID for the event.
    event_id TEXT PRIMARY KEY,
	-- The place of the event in the room's topology. This can usually be determined
	-- from the event's depth.
	topological_position BIGINT NOT NULL,
    -- The 'room_id' key for the event.
    room_id TEXT NOT NULL
);
-- The topological order will be used in events selection and ordering
CREATE UNIQUE INDEX IF NOT EXISTS syncapi_event_topological_position_idx ON syncapi_output_room_events_topology(topological_position, room_id);
`

const insertEventInTopologySQL = "" +
	"INSERT INTO syncapi_output_room_events_topology (event_id, topological_position, room_id)" +
	" VALUES ($1, $2, $3)" +
	" ON CONFLICT DO NOTHING"

const selectEventIDsInRangeASCSQL = "" +
	"SELECT event_id FROM syncapi_output_room_events_topology" +
	" WHERE room_id = $1 AND topological_position > $2 AND topological_position <= $3" +
	" ORDER BY topological_position ASC LIMIT $4"

const selectEventIDsInRangeDESCSQL = "" +
	"SELECT event_id FROM syncapi_output_room_events_topology" +
	" WHERE room_id = $1 AND topological_position > $2 AND topological_position <= $3" +
	" ORDER BY topological_position DESC LIMIT $4"

const selectPositionInTopologySQL = "" +
	"SELECT topological_position FROM syncapi_output_room_events_topology" +
	" WHERE event_id = $1"

const selectMaxPositionInTopologySQL = "" +
	"SELECT MAX(topological_position) FROM syncapi_output_room_events_topology" +
	" WHERE room_id = $1"

const selectEventIDsFromPositionSQL = "" +
	"SELECT event_id FROM syncapi_output_room_events_topology" +
	" WHERE room_id = $1 AND topological_position = $2"

type outputRoomEventsTopologyStatements struct {
	insertEventInTopologyStmt       *sql.Stmt
	selectEventIDsInRangeASCStmt    *sql.Stmt
	selectEventIDsInRangeDESCStmt   *sql.Stmt
	selectPositionInTopologyStmt    *sql.Stmt
	selectMaxPositionInTopologyStmt *sql.Stmt
	selectEventIDsFromPositionStmt  *sql.Stmt
}

func (s *outputRoomEventsTopologyStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(outputRoomEventsTopologySchema)
	if err != nil {
		return
	}
	if s.insertEventInTopologyStmt, err = db.Prepare(insertEventInTopologySQL); err != nil {
		return
	}
	if s.selectEventIDsInRangeASCStmt, err = db.Prepare(selectEventIDsInRangeASCSQL); err != nil {
		return
	}
	if s.selectEventIDsInRangeDESCStmt, err = db.Prepare(selectEventIDsInRangeDESCSQL); err != nil {
		return
	}
	if s.selectPositionInTopologyStmt, err = db.Prepare(selectPositionInTopologySQL); err != nil {
		return
	}
	if s.selectMaxPositionInTopologyStmt, err = db.Prepare(selectMaxPositionInTopologySQL); err != nil {
		return
	}
	if s.selectEventIDsFromPositionStmt, err = db.Prepare(selectEventIDsFromPositionSQL); err != nil {
		return
	}
	return
}

// insertEventInTopology inserts the given event in the room's topology, based
// on the event's depth.
func (s *outputRoomEventsTopologyStatements) insertEventInTopology(
	ctx context.Context, event *gomatrixserverlib.Event,
) (err error) {
	_, err = s.insertEventInTopologyStmt.ExecContext(
		ctx, event.EventID(), event.Depth(), event.RoomID(),
	)
	return
}

// selectEventIDsInRange selects the IDs of events which positions are within a
// given range in a given room's topological order.
// Returns an empty slice if no events match the given range.
func (s *outputRoomEventsTopologyStatements) selectEventIDsInRange(
	ctx context.Context, roomID string, fromPos, toPos types.StreamPosition,
	limit int, chronologicalOrder bool,
) (eventIDs []string, err error) {
	// Decide on the selection's order according to whether chronological order
	// is requested or not.
	var stmt *sql.Stmt
	if chronologicalOrder {
		stmt = s.selectEventIDsInRangeASCStmt
	} else {
		stmt = s.selectEventIDsInRangeDESCStmt
	}

	// Query the event IDs.
	rows, err := stmt.QueryContext(ctx, roomID, fromPos, toPos, limit)
	if err == sql.ErrNoRows {
		// If no event matched the request, return an empty slice.
		return []string{}, nil
	} else if err != nil {
		return
	}

	// Return the IDs.
	var eventID string
	for rows.Next() {
		if err = rows.Scan(&eventID); err != nil {
			return
		}
		eventIDs = append(eventIDs, eventID)
	}

	return
}

// selectPositionInTopology returns the position of a given event in the
// topology of the room it belongs to.
func (s *outputRoomEventsTopologyStatements) selectPositionInTopology(
	ctx context.Context, eventID string,
) (pos types.StreamPosition, err error) {
	err = s.selectPositionInTopologyStmt.QueryRowContext(ctx, eventID).Scan(&pos)
	return
}

func (s *outputRoomEventsTopologyStatements) selectMaxPositionInTopology(
	ctx context.Context, roomID string,
) (pos types.StreamPosition, err error) {
	err = s.selectMaxPositionInTopologyStmt.QueryRowContext(ctx, roomID).Scan(&pos)
	return
}

// selectEventIDsFromPosition returns the IDs of all events that have a given
// position in the topology of a given room.
func (s *outputRoomEventsTopologyStatements) selectEventIDsFromPosition(
	ctx context.Context, roomID string, pos types.StreamPosition,
) (eventIDs []string, err error) {
	// Query the event IDs.
	rows, err := s.selectEventIDsFromPositionStmt.QueryContext(ctx, roomID, pos)
	if err == sql.ErrNoRows {
		// If no event matched the request, return an empty slice.
		return []string{}, nil
	} else if err != nil {
		return
	}
	// Return the IDs.
	var eventID string
	for rows.Next() {
		if err = rows.Scan(&eventID); err != nil {
			return
		}
		eventIDs = append(eventIDs, eventID)
	}
	return
}
