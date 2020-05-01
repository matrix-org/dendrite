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

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/types"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
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
  event_id TEXT NOT NULL CONSTRAINT syncapi_event_id_idx UNIQUE,
  -- The 'room_id' key for the event.
  room_id TEXT NOT NULL,
  -- The headered JSON for the event, containing potentially additional metadata such as
  -- the room version. Stored as TEXT because this should be valid UTF-8.
  headered_event_json TEXT NOT NULL,
  -- The event type e.g 'm.room.member'.
  type TEXT NOT NULL,
  -- The 'sender' property of the event.
  sender TEXT NOT NULL,
  -- true if the event content contains a url key.
  contains_url BOOL NOT NULL,
  -- A list of event IDs which represent a delta of added/removed room state. This can be NULL
  -- if there is no delta.
  add_state_ids TEXT[],
  remove_state_ids TEXT[],
  -- The client session that sent the event, if any
  session_id BIGINT,
  -- The transaction id used to send the event, if any
  transaction_id TEXT,
  -- Should the event be excluded from responses to /sync requests. Useful for
  -- events retrieved through backfilling that have a position in the stream
  -- that relates to the moment these were retrieved rather than the moment these
  -- were emitted.
  exclude_from_sync BOOL DEFAULT FALSE
);
`

const insertEventSQL = "" +
	"INSERT INTO syncapi_output_room_events (" +
	"room_id, event_id, headered_event_json, type, sender, contains_url, add_state_ids, remove_state_ids, session_id, transaction_id, exclude_from_sync" +
	") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) " +
	"ON CONFLICT ON CONSTRAINT syncapi_event_id_idx DO UPDATE SET exclude_from_sync = $11 " +
	"RETURNING id"

const selectEventsSQL = "" +
	"SELECT id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events WHERE event_id = ANY($1)"

const selectRecentEventsSQL = "" +
	"SELECT id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events" +
	" WHERE room_id = $1 AND id > $2 AND id <= $3" +
	" ORDER BY id DESC LIMIT $4"

const selectRecentEventsForSyncSQL = "" +
	"SELECT id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events" +
	" WHERE room_id = $1 AND id > $2 AND id <= $3 AND exclude_from_sync = FALSE" +
	" ORDER BY id DESC LIMIT $4"

const selectEarlyEventsSQL = "" +
	"SELECT id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events" +
	" WHERE room_id = $1 AND id > $2 AND id <= $3" +
	" ORDER BY id ASC LIMIT $4"

const selectStreamPositionForEventIDSQL = "" +
	"SELECT id FROM syncapi_output_room_events WHERE event_id = $1"

const selectMaxEventIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_output_room_events"

// In order for us to apply the state updates correctly, rows need to be ordered in the order they were received (id).
const selectStateInRangeSQL = "" +
	"SELECT id, headered_event_json, exclude_from_sync, add_state_ids, remove_state_ids" +
	" FROM syncapi_output_room_events" +
	" WHERE (id > $1 AND id <= $2) AND (add_state_ids IS NOT NULL OR remove_state_ids IS NOT NULL)" +
	" AND ( $3::text[] IS NULL OR     sender  = ANY($3)  )" +
	" AND ( $4::text[] IS NULL OR NOT(sender  = ANY($4)) )" +
	" AND ( $5::text[] IS NULL OR     type LIKE ANY($5)  )" +
	" AND ( $6::text[] IS NULL OR NOT(type LIKE ANY($6)) )" +
	" AND ( $7::bool IS NULL   OR     contains_url = $7  )" +
	" ORDER BY id ASC" +
	" LIMIT $8"

type outputRoomEventsStatements struct {
	insertEventStmt                    *sql.Stmt
	selectEventsStmt                   *sql.Stmt
	selectMaxEventIDStmt               *sql.Stmt
	selectRecentEventsStmt             *sql.Stmt
	selectRecentEventsForSyncStmt      *sql.Stmt
	selectEarlyEventsStmt              *sql.Stmt
	selectStateInRangeStmt             *sql.Stmt
	selectStreamPositionForEventIDStmt *sql.Stmt
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
	if s.selectRecentEventsForSyncStmt, err = db.Prepare(selectRecentEventsForSyncSQL); err != nil {
		return
	}
	if s.selectEarlyEventsStmt, err = db.Prepare(selectEarlyEventsSQL); err != nil {
		return
	}
	if s.selectStateInRangeStmt, err = db.Prepare(selectStateInRangeSQL); err != nil {
		return
	}
	if s.selectStreamPositionForEventIDStmt, err = db.Prepare(selectStreamPositionForEventIDSQL); err != nil {
		return
	}
	return
}

func (s *outputRoomEventsStatements) selectStreamPositionForEventID(ctx context.Context, eventID string) (types.StreamPosition, error) {
	var id int64
	err := s.selectStreamPositionForEventIDStmt.QueryRowContext(ctx, eventID).Scan(&id)
	return types.StreamPosition(id), err
}

// selectStateInRange returns the state events between the two given PDU stream positions, exclusive of oldPos, inclusive of newPos.
// Results are bucketed based on the room ID. If the same state is overwritten multiple times between the
// two positions, only the most recent state is returned.
func (s *outputRoomEventsStatements) selectStateInRange(
	ctx context.Context, txn *sql.Tx, oldPos, newPos types.StreamPosition,
	stateFilter *gomatrixserverlib.StateFilter,
) (map[string]map[string]bool, map[string]types.StreamEvent, error) {
	stmt := common.TxStmt(txn, s.selectStateInRangeStmt)

	rows, err := stmt.QueryContext(
		ctx, oldPos, newPos,
		pq.StringArray(stateFilter.Senders),
		pq.StringArray(stateFilter.NotSenders),
		pq.StringArray(filterConvertTypeWildcardToSQL(stateFilter.Types)),
		pq.StringArray(filterConvertTypeWildcardToSQL(stateFilter.NotTypes)),
		stateFilter.ContainsURL,
		stateFilter.Limit,
	)
	if err != nil {
		return nil, nil, err
	}
	defer common.CloseAndLogIfError(ctx, rows, "selectStateInRange: rows.close() failed")
	// Fetch all the state change events for all rooms between the two positions then loop each event and:
	//  - Keep a cache of the event by ID (99% of state change events are for the event itself)
	//  - For each room ID, build up an array of event IDs which represents cumulative adds/removes
	// For each room, map cumulative event IDs to events and return. This may need to a batch SELECT based on event ID
	// if they aren't in the event ID cache. We don't handle state deletion yet.
	eventIDToEvent := make(map[string]types.StreamEvent)

	// RoomID => A set (map[string]bool) of state event IDs which are between the two positions
	stateNeeded := make(map[string]map[string]bool)

	for rows.Next() {
		var (
			streamPos       types.StreamPosition
			eventBytes      []byte
			excludeFromSync bool
			addIDs          pq.StringArray
			delIDs          pq.StringArray
		)
		if err := rows.Scan(&streamPos, &eventBytes, &excludeFromSync, &addIDs, &delIDs); err != nil {
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
		var ev gomatrixserverlib.HeaderedEvent
		if err := json.Unmarshal(eventBytes, &ev); err != nil {
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

		eventIDToEvent[ev.EventID()] = types.StreamEvent{
			HeaderedEvent:   ev,
			StreamPosition:  streamPos,
			ExcludeFromSync: excludeFromSync,
		}
	}

	return stateNeeded, eventIDToEvent, rows.Err()
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
	event *gomatrixserverlib.HeaderedEvent, addState, removeState []string,
	transactionID *api.TransactionID, excludeFromSync bool,
) (streamPos types.StreamPosition, err error) {
	var txnID *string
	var sessionID *int64
	if transactionID != nil {
		sessionID = &transactionID.SessionID
		txnID = &transactionID.TransactionID
	}

	// Parse content as JSON and search for an "url" key
	containsURL := false
	var content map[string]interface{}
	if json.Unmarshal(event.Content(), &content) != nil {
		// Set containsURL to true if url is present
		_, containsURL = content["url"]
	}

	var headeredJSON []byte
	headeredJSON, err = json.Marshal(event)
	if err != nil {
		return
	}

	stmt := common.TxStmt(txn, s.insertEventStmt)
	err = stmt.QueryRowContext(
		ctx,
		event.RoomID(),
		event.EventID(),
		headeredJSON,
		event.Type(),
		event.Sender(),
		containsURL,
		pq.StringArray(addState),
		pq.StringArray(removeState),
		sessionID,
		txnID,
		excludeFromSync,
	).Scan(&streamPos)
	return
}

// selectRecentEvents returns the most recent events in the given room, up to a maximum of 'limit'.
// If onlySyncEvents has a value of true, only returns the events that aren't marked as to exclude
// from sync.
func (s *outputRoomEventsStatements) selectRecentEvents(
	ctx context.Context, txn *sql.Tx,
	roomID string, fromPos, toPos types.StreamPosition, limit int,
	chronologicalOrder bool, onlySyncEvents bool,
) ([]types.StreamEvent, error) {
	var stmt *sql.Stmt
	if onlySyncEvents {
		stmt = common.TxStmt(txn, s.selectRecentEventsForSyncStmt)
	} else {
		stmt = common.TxStmt(txn, s.selectRecentEventsStmt)
	}
	rows, err := stmt.QueryContext(ctx, roomID, fromPos, toPos, limit)
	if err != nil {
		return nil, err
	}
	defer common.CloseAndLogIfError(ctx, rows, "selectRecentEvents: rows.close() failed")
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
) ([]types.StreamEvent, error) {
	stmt := common.TxStmt(txn, s.selectEarlyEventsStmt)
	rows, err := stmt.QueryContext(ctx, roomID, fromPos, toPos, limit)
	if err != nil {
		return nil, err
	}
	defer common.CloseAndLogIfError(ctx, rows, "selectEarlyEvents: rows.close() failed")
	events, err := rowsToStreamEvents(rows)
	if err != nil {
		return nil, err
	}
	// The events need to be returned from oldest to latest, which isn't
	// necessarily the way the SQL query returns them, so a sort is necessary to
	// ensure the events are in the right order in the slice.
	sort.SliceStable(events, func(i int, j int) bool {
		return events[i].StreamPosition < events[j].StreamPosition
	})
	return events, nil
}

// selectEvents returns the events for the given event IDs. If an event is
// missing from the database, it will be omitted.
func (s *outputRoomEventsStatements) selectEvents(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) ([]types.StreamEvent, error) {
	stmt := common.TxStmt(txn, s.selectEventsStmt)
	rows, err := stmt.QueryContext(ctx, pq.StringArray(eventIDs))
	if err != nil {
		return nil, err
	}
	defer common.CloseAndLogIfError(ctx, rows, "selectEvents: rows.close() failed")
	return rowsToStreamEvents(rows)
}

func rowsToStreamEvents(rows *sql.Rows) ([]types.StreamEvent, error) {
	var result []types.StreamEvent
	for rows.Next() {
		var (
			streamPos       types.StreamPosition
			eventBytes      []byte
			excludeFromSync bool
			sessionID       *int64
			txnID           *string
			transactionID   *api.TransactionID
		)
		if err := rows.Scan(&streamPos, &eventBytes, &sessionID, &excludeFromSync, &txnID); err != nil {
			return nil, err
		}
		// TODO: Handle redacted events
		var ev gomatrixserverlib.HeaderedEvent
		if err := json.Unmarshal(eventBytes, &ev); err != nil {
			return nil, err
		}

		if sessionID != nil && txnID != nil {
			transactionID = &api.TransactionID{
				SessionID:     *sessionID,
				TransactionID: *txnID,
			}
		}

		result = append(result, types.StreamEvent{
			HeaderedEvent:   ev,
			StreamPosition:  streamPos,
			TransactionID:   transactionID,
			ExcludeFromSync: excludeFromSync,
		})
	}
	return result, rows.Err()
}
