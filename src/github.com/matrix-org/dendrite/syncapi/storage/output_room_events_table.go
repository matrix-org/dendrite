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
	"sort"

	"github.com/matrix-org/dendrite/roomserver/api"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

const outputRoomEventsSchema = `
-- This sequence is shared between all the tables generated from kafka logs.
CREATE SEQUENCE IF NOT EXISTS syncapi_stream_id;

-- Stores output room events received from the roomserver.
CREATE TABLE IF NOT EXISTS syncapi_output_room_events (
    -- An incrementing ID which denotes the position in the log that this event resides at.
    -- NB: 'serial' makes no guarantees to increment by 1 every time, only that it increments.
    --     This isn't a problem for us since we just want to order by this field.
    id BIGINT PRIMARY KEY DEFAULT nextval('syncapi_stream_id'),
    -- The event ID for the event
    event_id TEXT NOT NULL,
    -- The 'room_id' key for the event.
    room_id TEXT NOT NULL,
    -- The JSON for the event. Stored as TEXT because this should be valid UTF-8.
    event_json TEXT NOT NULL,
    -- A list of event IDs which represent a delta of added/removed room state. This can be NULL
    -- if there is no delta.
    add_state_ids TEXT[],
    remove_state_ids TEXT[],
    device_id TEXT,  -- The local device that sent the event, if any
    transaction_id TEXT  -- The transaction id used to send the event, if any
);
-- for event selection
CREATE UNIQUE INDEX IF NOT EXISTS syncapi_event_id_idx ON syncapi_output_room_events(event_id);
`

const insertEventSQL = "" +
	"INSERT INTO syncapi_output_room_events (" +
	" room_id, event_id, event_json, add_state_ids, remove_state_ids, device_id, transaction_id" +
	") VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id"

const selectEventsSQL = "" +
	"SELECT id, event_json FROM syncapi_output_room_events WHERE event_id = ANY($1)"

const selectRecentEventsSQL = "" +
	"SELECT id, event_json, device_id, transaction_id FROM syncapi_output_room_events" +
	" WHERE room_id = $1 AND id > $2 AND id <= $3" +
	" ORDER BY id DESC LIMIT $4"

const selectEarlyEventsSQL = "" +
	"SELECT id, event_json, device_id, transaction_id FROM syncapi_output_room_events" +
	" WHERE room_id = $1 AND id > $2 AND id <= $3" +
	" ORDER BY id ASC LIMIT $4"

const selectMaxEventIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_output_room_events"

// In order for us to apply the state updates correctly, rows need to be ordered in the order they were received (id).
const selectStateInRangeSQL = "" +
	"SELECT id, event_json, add_state_ids, remove_state_ids" +
	" FROM syncapi_output_room_events" +
	" WHERE (id > $1 AND id <= $2) AND (add_state_ids IS NOT NULL OR remove_state_ids IS NOT NULL)" +
	" ORDER BY id ASC"

type outputRoomEventsStatements struct {
	insertEventStmt        *sql.Stmt
	selectEventsStmt       *sql.Stmt
	selectMaxEventIDStmt   *sql.Stmt
	selectRecentEventsStmt *sql.Stmt
	selectEarlyEventsStmt  *sql.Stmt
	selectStateInRangeStmt *sql.Stmt
}

func (s *outputRoomEventsStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(outputRoomEventsSchema)
	if err != nil {
		return
	}
	if s.insertEventStmt, err = db.Prepare(insertEventSQL); err != nil {
		return
	}
	if s.selectEventsStmt, err = db.Prepare(selectEventsSQL); err != nil {
		return
	}
	if s.selectMaxEventIDStmt, err = db.Prepare(selectMaxEventIDSQL); err != nil {
		return
	}
	if s.selectRecentEventsStmt, err = db.Prepare(selectRecentEventsSQL); err != nil {
		return
	}
	if s.selectEarlyEventsStmt, err = db.Prepare(selectEarlyEventsSQL); err != nil {
		return
	}
	if s.selectStateInRangeStmt, err = db.Prepare(selectStateInRangeSQL); err != nil {
		return
	}
	return
}

// selectStateInRange returns the state events between the two given stream positions, exclusive of oldPos, inclusive of newPos.
// Results are bucketed based on the room ID. If the same state is overwritten multiple times between the
// two positions, only the most recent state is returned.
func (s *outputRoomEventsStatements) selectStateInRange(
	ctx context.Context, txn *sql.Tx, oldPos, newPos types.StreamPosition,
) (map[string]map[string]bool, map[string]StreamEvent, error) {
	stmt := common.TxStmt(txn, s.selectStateInRangeStmt)

	rows, err := stmt.QueryContext(ctx, oldPos, newPos)
	if err != nil {
		return nil, nil, err
	}
	// Fetch all the state change events for all rooms between the two positions then loop each event and:
	//  - Keep a cache of the event by ID (99% of state change events are for the event itself)
	//  - For each room ID, build up an array of event IDs which represents cumulative adds/removes
	// For each room, map cumulative event IDs to events and return. This may need to a batch SELECT based on event ID
	// if they aren't in the event ID cache. We don't handle state deletion yet.
	eventIDToEvent := make(map[string]StreamEvent)

	// RoomID => A set (map[string]bool) of state event IDs which are between the two positions
	stateNeeded := make(map[string]map[string]bool)

	for rows.Next() {
		var (
			streamPos  int64
			eventBytes []byte
			addIDs     pq.StringArray
			delIDs     pq.StringArray
		)
		if err := rows.Scan(&streamPos, &eventBytes, &addIDs, &delIDs); err != nil {
			return nil, nil, err
		}
		// Sanity check for deleted state and whine if we see it. We don't need to do anything
		// since it'll just mark the event as not being needed.
		if len(addIDs) < len(delIDs) {
			log.WithFields(log.Fields{
				"since":   oldPos,
				"current": newPos,
				"adds":    addIDs,
				"dels":    delIDs,
			}).Warn("StateBetween: ignoring deleted state")
		}

		// TODO: Handle redacted events
		ev, err := gomatrixserverlib.NewEventFromTrustedJSON(eventBytes, false)
		if err != nil {
			return nil, nil, err
		}
		needSet := stateNeeded[ev.RoomID()]
		if needSet == nil { // make set if required
			needSet = make(map[string]bool)
		}
		for _, id := range delIDs {
			needSet[id] = false
		}
		for _, id := range addIDs {
			needSet[id] = true
		}
		stateNeeded[ev.RoomID()] = needSet

		eventIDToEvent[ev.EventID()] = StreamEvent{
			Event:          ev,
			StreamPosition: types.StreamPosition(streamPos),
		}
	}

	return stateNeeded, eventIDToEvent, nil
}

// MaxID returns the ID of the last inserted event in this table. 'txn' is optional. If it is not supplied,
// then this function should only ever be used at startup, as it will race with inserting events if it is
// done afterwards. If there are no inserted events, 0 is returned.
func (s *outputRoomEventsStatements) selectMaxEventID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64
	stmt := common.TxStmt(txn, s.selectMaxEventIDStmt)
	err = stmt.QueryRowContext(ctx).Scan(&nullableID)
	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}

// InsertEvent into the output_room_events table. addState and removeState are an optional list of state event IDs. Returns the position
// of the inserted event.
func (s *outputRoomEventsStatements) insertEvent(
	ctx context.Context, txn *sql.Tx,
	event *gomatrixserverlib.Event, addState, removeState []string,
	transactionID *api.TransactionID,
) (streamPos int64, err error) {
	var deviceID, txnID *string
	if transactionID != nil {
		deviceID = &transactionID.DeviceID
		txnID = &transactionID.TransactionID
	}

	stmt := common.TxStmt(txn, s.insertEventStmt)
	err = stmt.QueryRowContext(
		ctx,
		event.RoomID(),
		event.EventID(),
		event.JSON(),
		pq.StringArray(addState),
		pq.StringArray(removeState),
		deviceID,
		txnID,
	).Scan(&streamPos)
	return
}

// RecentEventsInRoom returns the most recent events in the given room, up to a maximum of 'limit'.
func (s *outputRoomEventsStatements) selectRecentEvents(
	ctx context.Context, txn *sql.Tx,
	roomID string, fromPos, toPos types.StreamPosition, limit int,
	chronologicalOrder bool,
) ([]StreamEvent, error) {
	stmt := common.TxStmt(txn, s.selectRecentEventsStmt)
	rows, err := stmt.QueryContext(ctx, roomID, fromPos, toPos, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	events, err := rowsToStreamEvents(rows)
	if err != nil {
		return nil, err
	}
	if chronologicalOrder {
		// The events need to be returned from oldest to latest, which isn't
		// necessary the way the SQL query returns them, so a sort is necessary to
		// ensure the events are in the right order in the slice.
		sort.SliceStable(events, func(i int, j int) bool {
			return events[i].StreamPosition < events[j].StreamPosition
		})
	}
	return events, nil
}

// selectEarlyEvents returns the earliest events in the given room, starting
// from a given position, up to a maximum of 'limit'.
func (s *outputRoomEventsStatements) selectEarlyEvents(
	ctx context.Context, txn *sql.Tx,
	roomID string, fromPos, toPos types.StreamPosition, limit int,
) ([]StreamEvent, error) {
	stmt := common.TxStmt(txn, s.selectEarlyEventsStmt)
	rows, err := stmt.QueryContext(ctx, roomID, fromPos, toPos, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	events, err := rowsToStreamEvents(rows)
	if err != nil {
		return nil, err
	}
	return events, nil
}

// Events returns the events for the given event IDs. Returns an error if any one of the event IDs given are missing
// from the database.
func (s *outputRoomEventsStatements) selectEvents(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) ([]StreamEvent, error) {
	stmt := common.TxStmt(txn, s.selectEventsStmt)
	rows, err := stmt.QueryContext(ctx, pq.StringArray(eventIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	return rowsToStreamEvents(rows)
}

func rowsToStreamEvents(rows *sql.Rows) ([]StreamEvent, error) {
	var result []StreamEvent
	for rows.Next() {
		var (
			streamPos     int64
			eventBytes    []byte
			deviceID      *string
			txnID         *string
			transactionID *api.TransactionID
		)
		if err := rows.Scan(&streamPos, &eventBytes, &deviceID, &txnID); err != nil {
			return nil, err
		}
		// TODO: Handle redacted events
		ev, err := gomatrixserverlib.NewEventFromTrustedJSON(eventBytes, false)
		if err != nil {
			return nil, err
		}

		if deviceID != nil && txnID != nil {
			transactionID = &api.TransactionID{
				DeviceID:      *deviceID,
				TransactionID: *txnID,
			}
		}

		result = append(result, StreamEvent{
			Event:          ev,
			StreamPosition: types.StreamPosition(streamPos),
			TransactionID:  transactionID,
		})
	}
	return result, nil
}
