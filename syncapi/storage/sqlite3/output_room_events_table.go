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
	"sort"

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
	"ON CONFLICT (event_id) DO UPDATE SET exclude_from_sync = $13"

const selectEventsSQL = "" +
	"SELECT id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events WHERE event_id = $1"

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

const selectMaxEventIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_output_room_events"

const updateEventJSONSQL = "" +
	"UPDATE syncapi_output_room_events SET headered_event_json=$1 WHERE event_id=$2"

// In order for us to apply the state updates correctly, rows need to be ordered in the order they were received (id).
/*
	$1 = oldPos,
	$2 = newPos,
	$3 = pq.StringArray(stateFilterPart.Senders),
	$4 = pq.StringArray(stateFilterPart.NotSenders),
	$5 = pq.StringArray(filterConvertTypeWildcardToSQL(stateFilterPart.Types)),
	$6 = pq.StringArray(filterConvertTypeWildcardToSQL(stateFilterPart.NotTypes)),
	$7 = stateFilterPart.ContainsURL,
	$8 = stateFilterPart.Limit,
*/
const selectStateInRangeSQL = "" +
	"SELECT id, headered_event_json, exclude_from_sync, add_state_ids, remove_state_ids" +
	" FROM syncapi_output_room_events" +
	" WHERE (id > $1 AND id <= $2)" + // old/new pos
	" AND (add_state_ids IS NOT NULL OR remove_state_ids IS NOT NULL)" +
	/*	" AND ( $3 IS NULL OR     sender  IN ($3)  )" + // sender
		" AND ( $4 IS NULL OR NOT(sender  IN ($4)) )" + // not sender
		" AND ( $5 IS NULL OR     type    IN ($5)  )" + // type
		" AND ( $6 IS NULL OR NOT(type    IN ($6)) )" + // not type
		" AND ( $7 IS NULL OR     contains_url = $7)" + // contains URL? */
	" ORDER BY id ASC" +
	" LIMIT $8" // limit

const deleteEventsForRoomSQL = "" +
	"DELETE FROM syncapi_output_room_events WHERE room_id = $1"

type outputRoomEventsStatements struct {
	db                            *sql.DB
	streamIDStatements            *streamIDStatements
	insertEventStmt               *sql.Stmt
	selectEventsStmt              *sql.Stmt
	selectMaxEventIDStmt          *sql.Stmt
	selectRecentEventsStmt        *sql.Stmt
	selectRecentEventsForSyncStmt *sql.Stmt
	selectEarlyEventsStmt         *sql.Stmt
	selectStateInRangeStmt        *sql.Stmt
	updateEventJSONStmt           *sql.Stmt
	deleteEventsForRoomStmt       *sql.Stmt
}

func NewSqliteEventsTable(db *sql.DB, streamID *streamIDStatements) (tables.Events, error) {
	s := &outputRoomEventsStatements{
		db:                 db,
		streamIDStatements: streamID,
	}
	_, err := db.Exec(outputRoomEventsSchema)
	if err != nil {
		return nil, err
	}
	if s.insertEventStmt, err = db.Prepare(insertEventSQL); err != nil {
		return nil, err
	}
	if s.selectEventsStmt, err = db.Prepare(selectEventsSQL); err != nil {
		return nil, err
	}
	if s.selectMaxEventIDStmt, err = db.Prepare(selectMaxEventIDSQL); err != nil {
		return nil, err
	}
	if s.selectRecentEventsStmt, err = db.Prepare(selectRecentEventsSQL); err != nil {
		return nil, err
	}
	if s.selectRecentEventsForSyncStmt, err = db.Prepare(selectRecentEventsForSyncSQL); err != nil {
		return nil, err
	}
	if s.selectEarlyEventsStmt, err = db.Prepare(selectEarlyEventsSQL); err != nil {
		return nil, err
	}
	if s.selectStateInRangeStmt, err = db.Prepare(selectStateInRangeSQL); err != nil {
		return nil, err
	}
	if s.updateEventJSONStmt, err = db.Prepare(updateEventJSONSQL); err != nil {
		return nil, err
	}
	if s.deleteEventsForRoomStmt, err = db.Prepare(deleteEventsForRoomSQL); err != nil {
		return nil, err
	}
	return s, nil
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
	stateFilterPart *gomatrixserverlib.StateFilter,
) (map[string]map[string]bool, map[string]types.StreamEvent, error) {
	stmt := sqlutil.TxStmt(txn, s.selectStateInRangeStmt)

	rows, err := stmt.QueryContext(
		ctx, r.Low(), r.High(),
		/*pq.StringArray(stateFilterPart.Senders),
		pq.StringArray(stateFilterPart.NotSenders),
		pq.StringArray(filterConvertTypeWildcardToSQL(stateFilterPart.Types)),
		pq.StringArray(filterConvertTypeWildcardToSQL(stateFilterPart.NotTypes)),
		stateFilterPart.ContainsURL,*/
		stateFilterPart.Limit,
	)
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
			streamPos       types.StreamPosition
			eventBytes      []byte
			excludeFromSync bool
			addIDsJSON      string
			delIDsJSON      string
		)
		if err := rows.Scan(&streamPos, &eventBytes, &excludeFromSync, &addIDsJSON, &delIDsJSON); err != nil {
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
	if json.Unmarshal(event.Content(), &content) != nil {
		// Set containsURL to true if url is present
		_, containsURL = content["url"]
	}

	var headeredJSON []byte
	headeredJSON, err := json.Marshal(event)
	if err != nil {
		return 0, err
	}

	addStateJSON, err := json.Marshal(addState)
	if err != nil {
		return 0, err
	}
	removeStateJSON, err := json.Marshal(removeState)
	if err != nil {
		return 0, err
	}

	streamPos, err := s.streamIDStatements.nextStreamID(ctx, txn)
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
	roomID string, r types.Range, limit int,
	chronologicalOrder bool, onlySyncEvents bool,
) ([]types.StreamEvent, bool, error) {
	var stmt *sql.Stmt
	if onlySyncEvents {
		stmt = sqlutil.TxStmt(txn, s.selectRecentEventsForSyncStmt)
	} else {
		stmt = sqlutil.TxStmt(txn, s.selectRecentEventsStmt)
	}

	rows, err := stmt.QueryContext(ctx, roomID, r.Low(), r.High(), limit+1)
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
	if len(events) > limit {
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
	roomID string, r types.Range, limit int,
) ([]types.StreamEvent, error) {
	stmt := sqlutil.TxStmt(txn, s.selectEarlyEventsStmt)
	rows, err := stmt.QueryContext(ctx, roomID, r.Low(), r.High(), limit)
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
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) ([]types.StreamEvent, error) {
	var returnEvents []types.StreamEvent
	stmt := sqlutil.TxStmt(txn, s.selectEventsStmt)
	for _, eventID := range eventIDs {
		rows, err := stmt.QueryContext(ctx, eventID)
		if err != nil {
			return nil, err
		}
		if streamEvents, err := rowsToStreamEvents(rows); err == nil {
			returnEvents = append(returnEvents, streamEvents...)
		}
		internal.CloseAndLogIfError(ctx, rows, "selectEvents: rows.close() failed")
	}
	return returnEvents, nil
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
	return result, nil
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
