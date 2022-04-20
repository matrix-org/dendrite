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
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

const outputRoomEventsSchema = `
-- Stores output room events received from the roomserver.
CREATE TABLE IF NOT EXISTS syncapi_output_room_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event_id TEXT NOT NULL UNIQUE,
  room_id TEXT NOT NULL,
  headered_event_json TEXT NOT NULL,
  type TEXT NOT NULL,
  sender TEXT NOT NULL,
  contains_url BOOL NOT NULL,
  add_state_ids TEXT, -- JSON encoded string array
  remove_state_ids TEXT, -- JSON encoded string array
  session_id BIGINT,
  transaction_id TEXT,
  exclude_from_sync BOOL NOT NULL DEFAULT FALSE
);
`

const insertEventSQL = "" +
	"INSERT INTO syncapi_output_room_events (" +
	"id, room_id, event_id, headered_event_json, type, sender, contains_url, add_state_ids, remove_state_ids, session_id, transaction_id, exclude_from_sync" +
	") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) " +
	"ON CONFLICT (event_id) DO UPDATE SET exclude_from_sync = (excluded.exclude_from_sync AND $13)"

const selectEventsSQL = "" +
	"SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events WHERE event_id IN ($1)"

const selectRecentEventsSQL = "" +
	"SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events" +
	" WHERE room_id = $1 AND id > $2 AND id <= $3"

// WHEN, ORDER BY and LIMIT are appended by prepareWithFilters

const selectRecentEventsForSyncSQL = "" +
	"SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events" +
	" WHERE room_id = $1 AND id > $2 AND id <= $3 AND exclude_from_sync = FALSE"

// WHEN, ORDER BY and LIMIT are appended by prepareWithFilters

const selectEarlyEventsSQL = "" +
	"SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events" +
	" WHERE room_id = $1 AND id > $2 AND id <= $3"

// WHEN, ORDER BY and LIMIT are appended by prepareWithFilters

const selectMaxEventIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_output_room_events"

const updateEventJSONSQL = "" +
	"UPDATE syncapi_output_room_events SET headered_event_json=$1 WHERE event_id=$2"

const selectStateInRangeSQL = "" +
	"SELECT event_id, id, headered_event_json, exclude_from_sync, add_state_ids, remove_state_ids" +
	" FROM syncapi_output_room_events" +
	" WHERE (id > $1 AND id <= $2)" +
	" AND room_id IN ($3)" +
	" AND ((add_state_ids IS NOT NULL AND add_state_ids != '') OR (remove_state_ids IS NOT NULL AND remove_state_ids != ''))"

// WHEN, ORDER BY and LIMIT are appended by prepareWithFilters

const deleteEventsForRoomSQL = "" +
	"DELETE FROM syncapi_output_room_events WHERE room_id = $1"

const selectContextEventSQL = "" +
	"SELECT id, headered_event_json FROM syncapi_output_room_events WHERE room_id = $1 AND event_id = $2"

const selectContextBeforeEventSQL = "" +
	"SELECT headered_event_json FROM syncapi_output_room_events WHERE room_id = $1 AND id < $2"

// WHEN, ORDER BY and LIMIT are appended by prepareWithFilters

const selectContextAfterEventSQL = "" +
	"SELECT id, headered_event_json FROM syncapi_output_room_events WHERE room_id = $1 AND id > $2"

// WHEN, ORDER BY and LIMIT are appended by prepareWithFilters

type outputRoomEventsStatements struct {
	db                           *sql.DB
	streamIDStatements           *StreamIDStatements
	insertEventStmt              *sql.Stmt
	selectMaxEventIDStmt         *sql.Stmt
	updateEventJSONStmt          *sql.Stmt
	deleteEventsForRoomStmt      *sql.Stmt
	selectContextEventStmt       *sql.Stmt
	selectContextBeforeEventStmt *sql.Stmt
	selectContextAfterEventStmt  *sql.Stmt
}

func NewSqliteEventsTable(db *sql.DB, streamID *StreamIDStatements) (tables.Events, error) {
	s := &outputRoomEventsStatements{
		db:                 db,
		streamIDStatements: streamID,
	}
	_, err := db.Exec(outputRoomEventsSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.insertEventStmt, insertEventSQL},
		{&s.selectMaxEventIDStmt, selectMaxEventIDSQL},
		{&s.updateEventJSONStmt, updateEventJSONSQL},
		{&s.deleteEventsForRoomStmt, deleteEventsForRoomSQL},
		{&s.selectContextEventStmt, selectContextEventSQL},
		{&s.selectContextBeforeEventStmt, selectContextBeforeEventSQL},
		{&s.selectContextAfterEventStmt, selectContextAfterEventSQL},
	}.Prepare(db)
}

func (s *outputRoomEventsStatements) UpdateEventJSON(ctx context.Context, event *gomatrixserverlib.HeaderedEvent) error {
	headeredJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = s.updateEventJSONStmt.ExecContext(ctx, headeredJSON, event.EventID())
	return err
}

// selectStateInRange returns the state events between the two given PDU stream positions, exclusive of oldPos, inclusive of newPos.
// Results are bucketed based on the room ID. If the same state is overwritten multiple times between the
// two positions, only the most recent state is returned.
func (s *outputRoomEventsStatements) SelectStateInRange(
	ctx context.Context, txn *sql.Tx, r types.Range,
	stateFilter *gomatrixserverlib.StateFilter, roomIDs []string,
) (map[string]map[string]bool, map[string]types.StreamEvent, error) {
	stmtSQL := strings.Replace(selectStateInRangeSQL, "($3)", sqlutil.QueryVariadicOffset(len(roomIDs), 2), 1)
	inputParams := []interface{}{
		r.Low(), r.High(),
	}
	for _, roomID := range roomIDs {
		inputParams = append(inputParams, roomID)
	}
	stmt, params, err := prepareWithFilters(
		s.db, txn, stmtSQL, inputParams,
		stateFilter.Senders, stateFilter.NotSenders,
		stateFilter.Types, stateFilter.NotTypes,
		nil, stateFilter.ContainsURL, stateFilter.Limit, FilterOrderAsc,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("s.prepareWithFilters: %w", err)
	}

	rows, err := stmt.QueryContext(ctx, params...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close() // nolint: errcheck
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
			eventID         string
			streamPos       types.StreamPosition
			eventBytes      []byte
			excludeFromSync bool
			addIDsJSON      string
			delIDsJSON      string
		)
		if err := rows.Scan(&eventID, &streamPos, &eventBytes, &excludeFromSync, &addIDsJSON, &delIDsJSON); err != nil {
			return nil, nil, err
		}

		addIDs, delIDs, err := unmarshalStateIDs(addIDsJSON, delIDsJSON)
		if err != nil {
			return nil, nil, err
		}

		// Sanity check for deleted state and whine if we see it. We don't need to do anything
		// since it'll just mark the event as not being needed.
		if len(addIDs) < len(delIDs) {
			log.WithFields(log.Fields{
				"since":   r.From,
				"current": r.To,
				"adds":    addIDsJSON,
				"dels":    delIDsJSON,
			}).Warn("StateBetween: ignoring deleted state")
		}

		// TODO: Handle redacted events
		var ev gomatrixserverlib.HeaderedEvent
		if err := ev.UnmarshalJSONWithEventID(eventBytes, eventID); err != nil {
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

		eventIDToEvent[eventID] = types.StreamEvent{
			HeaderedEvent:   &ev,
			StreamPosition:  streamPos,
			ExcludeFromSync: excludeFromSync,
		}
	}

	return stateNeeded, eventIDToEvent, nil
}

// MaxID returns the ID of the last inserted event in this table. 'txn' is optional. If it is not supplied,
// then this function should only ever be used at startup, as it will race with inserting events if it is
// done afterwards. If there are no inserted events, 0 is returned.
func (s *outputRoomEventsStatements) SelectMaxEventID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64
	stmt := sqlutil.TxStmt(txn, s.selectMaxEventIDStmt)
	err = stmt.QueryRowContext(ctx).Scan(&nullableID)
	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}

// InsertEvent into the output_room_events table. addState and removeState are an optional list of state event IDs. Returns the position
// of the inserted event.
func (s *outputRoomEventsStatements) InsertEvent(
	ctx context.Context, txn *sql.Tx,
	event *gomatrixserverlib.HeaderedEvent, addState, removeState []string,
	transactionID *api.TransactionID, excludeFromSync bool,
) (types.StreamPosition, error) {
	var txnID *string
	var sessionID *int64
	if transactionID != nil {
		sessionID = &transactionID.SessionID
		txnID = &transactionID.TransactionID
	}

	// Parse content as JSON and search for an "url" key
	containsURL := false
	var content map[string]interface{}
	if json.Unmarshal(event.Content(), &content) == nil {
		// Set containsURL to true if url is present
		_, containsURL = content["url"]
	}

	var headeredJSON []byte
	headeredJSON, err := json.Marshal(event)
	if err != nil {
		return 0, err
	}

	var addStateJSON, removeStateJSON []byte
	if len(addState) > 0 {
		addStateJSON, err = json.Marshal(addState)
	}
	if err != nil {
		return 0, fmt.Errorf("json.Marshal(addState): %w", err)
	}
	if len(removeState) > 0 {
		removeStateJSON, err = json.Marshal(removeState)
	}
	if err != nil {
		return 0, fmt.Errorf("json.Marshal(removeState): %w", err)
	}

	streamPos, err := s.streamIDStatements.nextPDUID(ctx, txn)
	if err != nil {
		return 0, err
	}
	insertStmt := sqlutil.TxStmt(txn, s.insertEventStmt)
	_, err = insertStmt.ExecContext(
		ctx,
		streamPos,
		event.RoomID(),
		event.EventID(),
		headeredJSON,
		event.Type(),
		event.Sender(),
		containsURL,
		string(addStateJSON),
		string(removeStateJSON),
		sessionID,
		txnID,
		excludeFromSync,
		excludeFromSync,
	)
	return streamPos, err
}

func (s *outputRoomEventsStatements) SelectRecentEvents(
	ctx context.Context, txn *sql.Tx,
	roomID string, r types.Range, eventFilter *gomatrixserverlib.RoomEventFilter,
	chronologicalOrder bool, onlySyncEvents bool,
) ([]types.StreamEvent, bool, error) {
	var query string
	if onlySyncEvents {
		query = selectRecentEventsForSyncSQL
	} else {
		query = selectRecentEventsSQL
	}

	stmt, params, err := prepareWithFilters(
		s.db, txn, query,
		[]interface{}{
			roomID, r.Low(), r.High(),
		},
		eventFilter.Senders, eventFilter.NotSenders,
		eventFilter.Types, eventFilter.NotTypes,
		nil, eventFilter.ContainsURL, eventFilter.Limit+1, FilterOrderDesc,
	)
	if err != nil {
		return nil, false, fmt.Errorf("s.prepareWithFilters: %w", err)
	}

	rows, err := stmt.QueryContext(ctx, params...)
	if err != nil {
		return nil, false, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectRecentEvents: rows.close() failed")
	events, err := rowsToStreamEvents(rows)
	if err != nil {
		return nil, false, err
	}
	if chronologicalOrder {
		// The events need to be returned from oldest to latest, which isn't
		// necessary the way the SQL query returns them, so a sort is necessary to
		// ensure the events are in the right order in the slice.
		sort.SliceStable(events, func(i int, j int) bool {
			return events[i].StreamPosition < events[j].StreamPosition
		})
	}
	// we queried for 1 more than the limit, so if we returned one more mark limited=true
	limited := false
	if len(events) > eventFilter.Limit {
		limited = true
		// re-slice the extra (oldest) event out: in chronological order this is the first entry, else the last.
		if chronologicalOrder {
			events = events[1:]
		} else {
			events = events[:len(events)-1]
		}
	}
	return events, limited, nil
}

func (s *outputRoomEventsStatements) SelectEarlyEvents(
	ctx context.Context, txn *sql.Tx,
	roomID string, r types.Range, eventFilter *gomatrixserverlib.RoomEventFilter,
) ([]types.StreamEvent, error) {
	stmt, params, err := prepareWithFilters(
		s.db, txn, selectEarlyEventsSQL,
		[]interface{}{
			roomID, r.Low(), r.High(),
		},
		eventFilter.Senders, eventFilter.NotSenders,
		eventFilter.Types, eventFilter.NotTypes,
		nil, eventFilter.ContainsURL, eventFilter.Limit, FilterOrderAsc,
	)
	if err != nil {
		return nil, fmt.Errorf("s.prepareWithFilters: %w", err)
	}
	rows, err := stmt.QueryContext(ctx, params...)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectEarlyEvents: rows.close() failed")
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
func (s *outputRoomEventsStatements) SelectEvents(
	ctx context.Context, txn *sql.Tx, eventIDs []string, filter *gomatrixserverlib.RoomEventFilter, preserveOrder bool,
) ([]types.StreamEvent, error) {
	iEventIDs := make([]interface{}, len(eventIDs))
	for i := range eventIDs {
		iEventIDs[i] = eventIDs[i]
	}
	selectSQL := strings.Replace(selectEventsSQL, "($1)", sqlutil.QueryVariadic(len(eventIDs)), 1)

	if filter == nil {
		filter = &gomatrixserverlib.RoomEventFilter{Limit: 20}
	}
	stmt, params, err := prepareWithFilters(
		s.db, txn, selectSQL, iEventIDs,
		filter.Senders, filter.NotSenders,
		filter.Types, filter.NotTypes,
		nil, filter.ContainsURL, filter.Limit, FilterOrderAsc,
	)
	if err != nil {
		return nil, err
	}
	rows, err := stmt.QueryContext(ctx, params...)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectEvents: rows.close() failed")
	streamEvents, err := rowsToStreamEvents(rows)
	if err != nil {
		return nil, err
	}
	if preserveOrder {
		var returnEvents []types.StreamEvent
		eventMap := make(map[string]types.StreamEvent)
		for _, ev := range streamEvents {
			eventMap[ev.EventID()] = ev
		}
		for _, eventID := range eventIDs {
			ev, ok := eventMap[eventID]
			if ok {
				returnEvents = append(returnEvents, ev)
			}
		}
		return returnEvents, nil
	}
	return streamEvents, nil
}

func (s *outputRoomEventsStatements) DeleteEventsForRoom(
	ctx context.Context, txn *sql.Tx, roomID string,
) (err error) {
	_, err = sqlutil.TxStmt(txn, s.deleteEventsForRoomStmt).ExecContext(ctx, roomID)
	return err
}

func rowsToStreamEvents(rows *sql.Rows) ([]types.StreamEvent, error) {
	var result []types.StreamEvent
	for rows.Next() {
		var (
			eventID         string
			streamPos       types.StreamPosition
			eventBytes      []byte
			excludeFromSync bool
			sessionID       *int64
			txnID           *string
			transactionID   *api.TransactionID
		)
		if err := rows.Scan(&eventID, &streamPos, &eventBytes, &sessionID, &excludeFromSync, &txnID); err != nil {
			return nil, err
		}
		// TODO: Handle redacted events
		var ev gomatrixserverlib.HeaderedEvent
		if err := ev.UnmarshalJSONWithEventID(eventBytes, eventID); err != nil {
			return nil, err
		}

		if sessionID != nil && txnID != nil {
			transactionID = &api.TransactionID{
				SessionID:     *sessionID,
				TransactionID: *txnID,
			}
		}

		result = append(result, types.StreamEvent{
			HeaderedEvent:   &ev,
			StreamPosition:  streamPos,
			TransactionID:   transactionID,
			ExcludeFromSync: excludeFromSync,
		})
	}
	return result, nil
}
func (s *outputRoomEventsStatements) SelectContextEvent(
	ctx context.Context, txn *sql.Tx, roomID, eventID string,
) (id int, evt gomatrixserverlib.HeaderedEvent, err error) {
	row := sqlutil.TxStmt(txn, s.selectContextEventStmt).QueryRowContext(ctx, roomID, eventID)
	var eventAsString string
	if err = row.Scan(&id, &eventAsString); err != nil {
		return 0, evt, err
	}

	if err = json.Unmarshal([]byte(eventAsString), &evt); err != nil {
		return 0, evt, err
	}
	return id, evt, nil
}

func (s *outputRoomEventsStatements) SelectContextBeforeEvent(
	ctx context.Context, txn *sql.Tx, id int, roomID string, filter *gomatrixserverlib.RoomEventFilter,
) (evts []*gomatrixserverlib.HeaderedEvent, err error) {
	stmt, params, err := prepareWithFilters(
		s.db, txn, selectContextBeforeEventSQL,
		[]interface{}{
			roomID, id,
		},
		filter.Senders, filter.NotSenders,
		filter.Types, filter.NotTypes,
		nil, filter.ContainsURL, filter.Limit, FilterOrderDesc,
	)

	rows, err := stmt.QueryContext(ctx, params...)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "rows.close() failed")

	for rows.Next() {
		var (
			eventBytes []byte
			evt        *gomatrixserverlib.HeaderedEvent
		)
		if err = rows.Scan(&eventBytes); err != nil {
			return evts, err
		}
		if err = json.Unmarshal(eventBytes, &evt); err != nil {
			return evts, err
		}
		evts = append(evts, evt)
	}

	return evts, rows.Err()
}

func (s *outputRoomEventsStatements) SelectContextAfterEvent(
	ctx context.Context, txn *sql.Tx, id int, roomID string, filter *gomatrixserverlib.RoomEventFilter,
) (lastID int, evts []*gomatrixserverlib.HeaderedEvent, err error) {
	stmt, params, err := prepareWithFilters(
		s.db, txn, selectContextAfterEventSQL,
		[]interface{}{
			roomID, id,
		},
		filter.Senders, filter.NotSenders,
		filter.Types, filter.NotTypes,
		nil, filter.ContainsURL, filter.Limit, FilterOrderAsc,
	)

	rows, err := stmt.QueryContext(ctx, params...)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "rows.close() failed")

	for rows.Next() {
		var (
			eventBytes []byte
			evt        *gomatrixserverlib.HeaderedEvent
		)
		if err = rows.Scan(&lastID, &eventBytes); err != nil {
			return 0, evts, err
		}
		if err = json.Unmarshal(eventBytes, &evt); err != nil {
			return 0, evts, err
		}
		evts = append(evts, evt)
	}
	return lastID, evts, rows.Err()
}

func unmarshalStateIDs(addIDsJSON, delIDsJSON string) (addIDs []string, delIDs []string, err error) {
	if len(addIDsJSON) > 0 {
		if err = json.Unmarshal([]byte(addIDsJSON), &addIDs); err != nil {
			return
		}
	}
	if len(delIDsJSON) > 0 {
		if err = json.Unmarshal([]byte(delIDsJSON), &delIDs); err != nil {
			return
		}
	}
	return
}
