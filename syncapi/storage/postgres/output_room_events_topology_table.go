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

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	rstypes "github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
)

const outputRoomEventsTopologySchema = `
-- Stores output room events received from the roomserver.
CREATE TABLE IF NOT EXISTS syncapi_output_room_events_topology (
	-- The event ID for the event.
    event_id TEXT PRIMARY KEY,
	-- The place of the event in the room's topology. This can usually be determined
	-- from the event's depth.
	topological_position BIGINT NOT NULL,
	stream_position BIGINT NOT NULL,
    -- The 'room_id' key for the event.
    room_id TEXT NOT NULL
);
-- The topological order will be used in events selection and ordering
CREATE UNIQUE INDEX IF NOT EXISTS syncapi_event_topological_position_idx ON syncapi_output_room_events_topology(topological_position, stream_position, room_id);
`

const insertEventInTopologySQL = "" +
	"INSERT INTO syncapi_output_room_events_topology (event_id, topological_position, room_id, stream_position)" +
	" VALUES ($1, $2, $3, $4)" +
	" ON CONFLICT (topological_position, stream_position, room_id) DO UPDATE SET event_id = $1" +
	" RETURNING topological_position"

const selectEventIDsInRangeASCSQL = "" +
	"SELECT event_id, topological_position, stream_position FROM syncapi_output_room_events_topology" +
	" WHERE room_id = $1 AND (" +
	"(topological_position > $2 AND topological_position < $3) OR" +
	"(topological_position = $4 AND stream_position >= $5)" +
	") ORDER BY topological_position ASC, stream_position ASC LIMIT $6"

const selectEventIDsInRangeDESCSQL = "" +
	"SELECT event_id, topological_position, stream_position FROM syncapi_output_room_events_topology" +
	" WHERE room_id = $1 AND (" +
	"(topological_position > $2 AND topological_position < $3) OR" +
	"(topological_position = $4 AND stream_position <= $5)" +
	") ORDER BY topological_position DESC, stream_position DESC LIMIT $6"

const selectPositionInTopologySQL = "" +
	"SELECT topological_position, stream_position FROM syncapi_output_room_events_topology" +
	" WHERE event_id = $1"

const selectStreamToTopologicalPositionAscSQL = "" +
	"SELECT topological_position FROM syncapi_output_room_events_topology WHERE room_id = $1 AND stream_position >= $2 ORDER BY topological_position ASC LIMIT 1;"

const selectStreamToTopologicalPositionDescSQL = "" +
	"SELECT topological_position FROM syncapi_output_room_events_topology WHERE room_id = $1 AND stream_position <= $2 ORDER BY topological_position DESC LIMIT 1;"

const purgeEventsTopologySQL = "" +
	"DELETE FROM syncapi_output_room_events_topology WHERE room_id = $1"

type outputRoomEventsTopologyStatements struct {
	insertEventInTopologyStmt                 *sql.Stmt
	selectEventIDsInRangeASCStmt              *sql.Stmt
	selectEventIDsInRangeDESCStmt             *sql.Stmt
	selectPositionInTopologyStmt              *sql.Stmt
	selectStreamToTopologicalPositionAscStmt  *sql.Stmt
	selectStreamToTopologicalPositionDescStmt *sql.Stmt
	purgeEventsTopologyStmt                   *sql.Stmt
}

func NewPostgresTopologyTable(db *sql.DB) (tables.Topology, error) {
	s := &outputRoomEventsTopologyStatements{}
	_, err := db.Exec(outputRoomEventsTopologySchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.insertEventInTopologyStmt, insertEventInTopologySQL},
		{&s.selectEventIDsInRangeASCStmt, selectEventIDsInRangeASCSQL},
		{&s.selectEventIDsInRangeDESCStmt, selectEventIDsInRangeDESCSQL},
		{&s.selectPositionInTopologyStmt, selectPositionInTopologySQL},
		{&s.selectStreamToTopologicalPositionAscStmt, selectStreamToTopologicalPositionAscSQL},
		{&s.selectStreamToTopologicalPositionDescStmt, selectStreamToTopologicalPositionDescSQL},
		{&s.purgeEventsTopologyStmt, purgeEventsTopologySQL},
	}.Prepare(db)
}

// InsertEventInTopology inserts the given event in the room's topology, based
// on the event's depth.
func (s *outputRoomEventsTopologyStatements) InsertEventInTopology(
	ctx context.Context, txn *sql.Tx, event *rstypes.HeaderedEvent, pos types.StreamPosition,
) (topoPos types.StreamPosition, err error) {
	err = sqlutil.TxStmt(txn, s.insertEventInTopologyStmt).QueryRowContext(
		ctx, event.EventID(), event.Depth(), event.RoomID(), pos,
	).Scan(&topoPos)
	return
}

// SelectEventIDsInRange selects the IDs of events which positions are within a
// given range in a given room's topological order. Returns the start/end topological tokens for
// the returned eventIDs.
// Returns an empty slice if no events match the given range.
func (s *outputRoomEventsTopologyStatements) SelectEventIDsInRange(
	ctx context.Context, txn *sql.Tx, roomID string, minDepth, maxDepth, maxStreamPos types.StreamPosition,
	limit int, chronologicalOrder bool,
) (eventIDs []string, start, end types.TopologyToken, err error) {
	// Decide on the selection's order according to whether chronological order
	// is requested or not.
	var stmt *sql.Stmt
	if chronologicalOrder {
		stmt = sqlutil.TxStmt(txn, s.selectEventIDsInRangeASCStmt)
	} else {
		stmt = sqlutil.TxStmt(txn, s.selectEventIDsInRangeDESCStmt)
	}

	// Query the event IDs.
	rows, err := stmt.QueryContext(ctx, roomID, minDepth, maxDepth, maxDepth, maxStreamPos, limit)
	if err == sql.ErrNoRows {
		// If no event matched the request, return an empty slice.
		return []string{}, start, end, nil
	} else if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectEventIDsInRange: rows.close() failed")

	// Return the IDs.
	var eventID string
	var token types.TopologyToken
	var tokens []types.TopologyToken
	for rows.Next() {
		if err = rows.Scan(&eventID, &token.Depth, &token.PDUPosition); err != nil {
			return
		}
		eventIDs = append(eventIDs, eventID)
		tokens = append(tokens, token)
	}

	// The values are already ordered by SQL, so we can use them as is.
	if len(tokens) > 0 {
		start = tokens[0]
		end = tokens[len(tokens)-1]
	}

	return eventIDs, start, end, rows.Err()
}

// SelectPositionInTopology returns the position of a given event in the
// topology of the room it belongs to.
func (s *outputRoomEventsTopologyStatements) SelectPositionInTopology(
	ctx context.Context, txn *sql.Tx, eventID string,
) (pos, spos types.StreamPosition, err error) {
	err = sqlutil.TxStmt(txn, s.selectPositionInTopologyStmt).QueryRowContext(ctx, eventID).Scan(&pos, &spos)
	return
}

// SelectStreamToTopologicalPosition returns the closest position of a given event
// in the topology of the room it belongs to from the given stream position.
func (s *outputRoomEventsTopologyStatements) SelectStreamToTopologicalPosition(
	ctx context.Context, txn *sql.Tx, roomID string, streamPos types.StreamPosition, backwardOrdering bool,
) (topoPos types.StreamPosition, err error) {
	if backwardOrdering {
		err = sqlutil.TxStmt(txn, s.selectStreamToTopologicalPositionDescStmt).QueryRowContext(ctx, roomID, streamPos).Scan(&topoPos)
	} else {
		err = sqlutil.TxStmt(txn, s.selectStreamToTopologicalPositionAscStmt).QueryRowContext(ctx, roomID, streamPos).Scan(&topoPos)
	}
	return
}

func (s *outputRoomEventsTopologyStatements) PurgeEventsTopology(
	ctx context.Context, txn *sql.Tx, roomID string,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeEventsTopologyStmt).ExecContext(ctx, roomID)
	return err
}
