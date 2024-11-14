// Copyright 2024 New Vector Ltd.
// Copyright 2019, 2020 The Matrix.org Foundation C.I.C.
// Copyright 2017, 2018 New Vector Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/roomserver/api"
	rstypes "github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/syncapi/storage/postgres/deltas"
	"github.com/element-hq/dendrite/syncapi/storage/tables"
	"github.com/element-hq/dendrite/syncapi/synctypes"
	"github.com/element-hq/dendrite/syncapi/types"
	"github.com/lib/pq"
	"github.com/matrix-org/gomatrixserverlib"
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
  event_id TEXT NOT NULL CONSTRAINT syncapi_output_room_event_id_idx UNIQUE,
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
  exclude_from_sync BOOL DEFAULT FALSE,
  -- The history visibility before this event (1 - world_readable; 2 - shared; 3 - invited; 4 - joined)
  history_visibility SMALLINT NOT NULL DEFAULT 2
);

CREATE INDEX IF NOT EXISTS syncapi_output_room_events_type_idx ON syncapi_output_room_events (type);
CREATE INDEX IF NOT EXISTS syncapi_output_room_events_sender_idx ON syncapi_output_room_events (sender);
CREATE INDEX IF NOT EXISTS syncapi_output_room_events_room_id_idx ON syncapi_output_room_events (room_id);
CREATE INDEX IF NOT EXISTS syncapi_output_room_events_exclude_from_sync_idx ON syncapi_output_room_events (exclude_from_sync);
CREATE INDEX IF NOT EXISTS syncapi_output_room_events_add_state_ids_idx ON syncapi_output_room_events ((add_state_ids IS NOT NULL));
CREATE INDEX IF NOT EXISTS syncapi_output_room_events_remove_state_ids_idx ON syncapi_output_room_events ((remove_state_ids IS NOT NULL));
CREATE INDEX IF NOT EXISTS syncapi_output_room_events_recent_events_idx ON syncapi_output_room_events (room_id, exclude_from_sync, id, sender, type);


`

const insertEventSQL = "" +
	"INSERT INTO syncapi_output_room_events (" +
	"room_id, event_id, headered_event_json, type, sender, contains_url, add_state_ids, remove_state_ids, session_id, transaction_id, exclude_from_sync, history_visibility" +
	") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) " +
	"ON CONFLICT ON CONSTRAINT syncapi_output_room_event_id_idx DO UPDATE SET exclude_from_sync = (excluded.exclude_from_sync AND $11) " +
	"RETURNING id"

const selectEventsSQL = "" +
	"SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id, history_visibility FROM syncapi_output_room_events WHERE event_id = ANY($1)"

const selectEventsWithFilterSQL = "" +
	"SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id, history_visibility FROM syncapi_output_room_events WHERE event_id = ANY($1)" +
	" AND ( $2::text[] IS NULL OR     sender  = ANY($2)  )" +
	" AND ( $3::text[] IS NULL OR NOT(sender  = ANY($3)) )" +
	" AND ( $4::text[] IS NULL OR     type LIKE ANY($4)  )" +
	" AND ( $5::text[] IS NULL OR NOT(type LIKE ANY($5)) )" +
	" AND ( $6::bool   IS NULL OR     contains_url = $6 )" +
	" LIMIT $7"

const selectRecentEventsSQL = "" +
	"SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id, history_visibility FROM syncapi_output_room_events" +
	" WHERE room_id = $1 AND id > $2 AND id <= $3" +
	" AND ( $4::text[] IS NULL OR     sender  = ANY($4)  )" +
	" AND ( $5::text[] IS NULL OR NOT(sender  = ANY($5)) )" +
	" AND ( $6::text[] IS NULL OR     type LIKE ANY($6)  )" +
	" AND ( $7::text[] IS NULL OR NOT(type LIKE ANY($7)) )" +
	" ORDER BY id DESC LIMIT $8"

// selectRecentEventsForSyncSQL contains an optimization to get the recent events for a list of rooms, using a LATERAL JOIN
// The sub select inside LATERAL () is executed for all room_ids it gets as a parameter $1
const selectRecentEventsForSyncSQL = `
WITH room_ids AS (
     SELECT unnest($1::text[]) AS room_id
)
SELECT    x.*
FROM room_ids,
          LATERAL  (
              SELECT room_id, event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id, history_visibility
                    FROM syncapi_output_room_events recent_events
                    WHERE
                      recent_events.room_id = room_ids.room_id
                      AND recent_events.exclude_from_sync = FALSE
                      AND id > $2 AND id <= $3
                      AND ( $4::text[] IS NULL OR     sender  = ANY($4)  )
                      AND ( $5::text[] IS NULL OR NOT(sender  = ANY($5)) )
                      AND ( $6::text[] IS NULL OR     type LIKE ANY($6)  )
                      AND ( $7::text[] IS NULL OR NOT(type LIKE ANY($7)) )
                    ORDER BY recent_events.id DESC
                    LIMIT $8
              ) AS x
`

const selectMaxEventIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_output_room_events"

const updateEventJSONSQL = "" +
	"UPDATE syncapi_output_room_events SET headered_event_json=$1 WHERE event_id=$2"

// In order for us to apply the state updates correctly, rows need to be ordered in the order they were received (id).
const selectStateInRangeFilteredSQL = "" +
	"SELECT event_id, id, headered_event_json, exclude_from_sync, add_state_ids, remove_state_ids, history_visibility" +
	" FROM syncapi_output_room_events" +
	" WHERE (id > $1 AND id <= $2) AND (add_state_ids IS NOT NULL OR remove_state_ids IS NOT NULL)" +
	" AND room_id = ANY($3)" +
	" AND ( $4::text[] IS NULL OR     sender  = ANY($4)  )" +
	" AND ( $5::text[] IS NULL OR NOT(sender  = ANY($5)) )" +
	" AND ( $6::text[] IS NULL OR     type LIKE ANY($6)  )" +
	" AND ( $7::text[] IS NULL OR NOT(type LIKE ANY($7)) )" +
	" AND ( $8::bool IS NULL   OR     contains_url = $8  )" +
	" ORDER BY id ASC"

// In order for us to apply the state updates correctly, rows need to be ordered in the order they were received (id).
const selectStateInRangeSQL = "" +
	"SELECT event_id, id, headered_event_json, exclude_from_sync, add_state_ids, remove_state_ids, history_visibility" +
	" FROM syncapi_output_room_events" +
	" WHERE (id > $1 AND id <= $2) AND (add_state_ids IS NOT NULL OR remove_state_ids IS NOT NULL)" +
	" AND room_id = ANY($3)" +
	" ORDER BY id ASC"

const deleteEventsForRoomSQL = "" +
	"DELETE FROM syncapi_output_room_events WHERE room_id = $1"

const selectContextEventSQL = "" +
	"SELECT id, headered_event_json, history_visibility FROM syncapi_output_room_events WHERE room_id = $1 AND event_id = $2"

const selectContextBeforeEventSQL = "" +
	"SELECT headered_event_json, history_visibility FROM syncapi_output_room_events WHERE room_id = $1 AND id < $2" +
	" AND ( $4::text[] IS NULL OR     sender  = ANY($4)  )" +
	" AND ( $5::text[] IS NULL OR NOT(sender  = ANY($5)) )" +
	" AND ( $6::text[] IS NULL OR     type LIKE ANY($6)  )" +
	" AND ( $7::text[] IS NULL OR NOT(type LIKE ANY($7)) )" +
	" ORDER BY id DESC LIMIT $3"

const selectContextAfterEventSQL = "" +
	"SELECT id, headered_event_json, history_visibility FROM syncapi_output_room_events WHERE room_id = $1 AND id > $2" +
	" AND ( $4::text[] IS NULL OR     sender  = ANY($4)  )" +
	" AND ( $5::text[] IS NULL OR NOT(sender  = ANY($5)) )" +
	" AND ( $6::text[] IS NULL OR     type LIKE ANY($6)  )" +
	" AND ( $7::text[] IS NULL OR NOT(type LIKE ANY($7)) )" +
	" ORDER BY id ASC LIMIT $3"

const purgeEventsSQL = "" +
	"DELETE FROM syncapi_output_room_events WHERE room_id = $1"

const selectSearchSQL = "SELECT id, event_id, headered_event_json FROM syncapi_output_room_events WHERE id > $1 AND type = ANY($2) ORDER BY id ASC LIMIT $3"

type outputRoomEventsStatements struct {
	insertEventStmt                *sql.Stmt
	selectEventsStmt               *sql.Stmt
	selectEventsWitFilterStmt      *sql.Stmt
	selectMaxEventIDStmt           *sql.Stmt
	selectRecentEventsStmt         *sql.Stmt
	selectRecentEventsForSyncStmt  *sql.Stmt
	selectStateInRangeFilteredStmt *sql.Stmt
	selectStateInRangeStmt         *sql.Stmt
	updateEventJSONStmt            *sql.Stmt
	deleteEventsForRoomStmt        *sql.Stmt
	selectContextEventStmt         *sql.Stmt
	selectContextBeforeEventStmt   *sql.Stmt
	selectContextAfterEventStmt    *sql.Stmt
	purgeEventsStmt                *sql.Stmt
	selectSearchStmt               *sql.Stmt
}

func NewPostgresEventsTable(db *sql.DB) (tables.Events, error) {
	s := &outputRoomEventsStatements{}
	_, err := db.Exec(outputRoomEventsSchema)
	if err != nil {
		return nil, err
	}

	migrationName := "syncapi: rename dupe index (output_room_events)"

	var cName string
	err = db.QueryRowContext(context.Background(), "select constraint_name from information_schema.table_constraints where table_name = 'syncapi_output_room_events' AND constraint_name = 'syncapi_event_id_idx'").Scan(&cName)
	switch err {
	case sql.ErrNoRows: // migration was already executed, as the index was renamed
		if err = sqlutil.InsertMigration(context.Background(), db, migrationName); err != nil {
			return nil, fmt.Errorf("unable to manually insert migration '%s': %w", migrationName, err)
		}
	case nil:
	default:
		return nil, err
	}

	m := sqlutil.NewMigrator(db)
	m.AddMigrations(
		sqlutil.Migration{
			Version: "syncapi: add history visibility column (output_room_events)",
			Up:      deltas.UpAddHistoryVisibilityColumnOutputRoomEvents,
		},
		sqlutil.Migration{
			Version: migrationName,
			Up:      deltas.UpRenameOutputRoomEventsIndex,
		},
	)
	err = m.Up(context.Background())
	if err != nil {
		return nil, err
	}

	return s, sqlutil.StatementList{
		{&s.insertEventStmt, insertEventSQL},
		{&s.selectEventsStmt, selectEventsSQL},
		{&s.selectEventsWitFilterStmt, selectEventsWithFilterSQL},
		{&s.selectMaxEventIDStmt, selectMaxEventIDSQL},
		{&s.selectRecentEventsStmt, selectRecentEventsSQL},
		{&s.selectRecentEventsForSyncStmt, selectRecentEventsForSyncSQL},
		{&s.selectStateInRangeFilteredStmt, selectStateInRangeFilteredSQL},
		{&s.selectStateInRangeStmt, selectStateInRangeSQL},
		{&s.updateEventJSONStmt, updateEventJSONSQL},
		{&s.deleteEventsForRoomStmt, deleteEventsForRoomSQL},
		{&s.selectContextEventStmt, selectContextEventSQL},
		{&s.selectContextBeforeEventStmt, selectContextBeforeEventSQL},
		{&s.selectContextAfterEventStmt, selectContextAfterEventSQL},
		{&s.purgeEventsStmt, purgeEventsSQL},
		{&s.selectSearchStmt, selectSearchSQL},
	}.Prepare(db)
}

func (s *outputRoomEventsStatements) UpdateEventJSON(ctx context.Context, txn *sql.Tx, event *rstypes.HeaderedEvent) error {
	headeredJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = sqlutil.TxStmt(txn, s.updateEventJSONStmt).ExecContext(ctx, headeredJSON, event.EventID())
	return err
}

// selectStateInRange returns the state events between the two given PDU stream positions, exclusive of oldPos, inclusive of newPos.
// Results are bucketed based on the room ID. If the same state is overwritten multiple times between the
// two positions, only the most recent state is returned.
func (s *outputRoomEventsStatements) SelectStateInRange(
	ctx context.Context, txn *sql.Tx, r types.Range,
	stateFilter *synctypes.StateFilter, roomIDs []string,
) (map[string]map[string]bool, map[string]types.StreamEvent, error) {
	var rows *sql.Rows
	var err error
	if stateFilter != nil {
		stmt := sqlutil.TxStmt(txn, s.selectStateInRangeFilteredStmt)
		senders, notSenders := getSendersStateFilterFilter(stateFilter)
		rows, err = stmt.QueryContext(
			ctx, r.Low(), r.High(), pq.StringArray(roomIDs),
			pq.StringArray(senders),
			pq.StringArray(notSenders),
			pq.StringArray(filterConvertTypeWildcardToSQL(stateFilter.Types)),
			pq.StringArray(filterConvertTypeWildcardToSQL(stateFilter.NotTypes)),
			stateFilter.ContainsURL,
		)
	} else {
		stmt := sqlutil.TxStmt(txn, s.selectStateInRangeStmt)
		rows, err = stmt.QueryContext(
			ctx, r.Low(), r.High(), pq.StringArray(roomIDs),
		)
	}

	if err != nil {
		return nil, nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectStateInRange: rows.close() failed")
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
			eventID           string
			streamPos         types.StreamPosition
			eventBytes        []byte
			excludeFromSync   bool
			addIDs            pq.StringArray
			delIDs            pq.StringArray
			historyVisibility gomatrixserverlib.HistoryVisibility
		)
		if err := rows.Scan(&eventID, &streamPos, &eventBytes, &excludeFromSync, &addIDs, &delIDs, &historyVisibility); err != nil {
			return nil, nil, err
		}

		// TODO: Handle redacted events
		var ev rstypes.HeaderedEvent
		if err := json.Unmarshal(eventBytes, &ev); err != nil {
			return nil, nil, err
		}
		needSet := stateNeeded[ev.RoomID().String()]
		if needSet == nil { // make set if required
			needSet = make(map[string]bool)
		}
		for _, id := range delIDs {
			needSet[id] = false
		}
		for _, id := range addIDs {
			needSet[id] = true
		}
		stateNeeded[ev.RoomID().String()] = needSet
		ev.Visibility = historyVisibility

		eventIDToEvent[eventID] = types.StreamEvent{
			HeaderedEvent:   &ev,
			StreamPosition:  streamPos,
			ExcludeFromSync: excludeFromSync,
		}
	}

	return stateNeeded, eventIDToEvent, rows.Err()
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
	event *rstypes.HeaderedEvent, addState, removeState []string,
	transactionID *api.TransactionID, excludeFromSync bool, historyVisibility gomatrixserverlib.HistoryVisibility,
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
	if json.Unmarshal(event.Content(), &content) == nil {
		// Set containsURL to true if url is present
		_, containsURL = content["url"]
	}

	var headeredJSON []byte
	headeredJSON, err = json.Marshal(event)
	if err != nil {
		return
	}

	stmt := sqlutil.TxStmt(txn, s.insertEventStmt)
	err = stmt.QueryRowContext(
		ctx,
		event.RoomID().String(),
		event.EventID(),
		headeredJSON,
		event.Type(),
		event.UserID.String(),
		containsURL,
		pq.StringArray(addState),
		pq.StringArray(removeState),
		sessionID,
		txnID,
		excludeFromSync,
		historyVisibility,
	).Scan(&streamPos)
	return
}

// selectRecentEvents returns the most recent events in the given room, up to a maximum of 'limit'.
// If onlySyncEvents has a value of true, only returns the events that aren't marked as to exclude
// from sync.
func (s *outputRoomEventsStatements) SelectRecentEvents(
	ctx context.Context, txn *sql.Tx,
	roomIDs []string, ra types.Range, eventFilter *synctypes.RoomEventFilter,
	chronologicalOrder bool, onlySyncEvents bool,
) (map[string]types.RecentEvents, error) {
	var stmt *sql.Stmt
	if onlySyncEvents {
		stmt = sqlutil.TxStmt(txn, s.selectRecentEventsForSyncStmt)
	} else {
		stmt = sqlutil.TxStmt(txn, s.selectRecentEventsStmt)
	}
	senders, notSenders := getSendersRoomEventFilter(eventFilter)

	rows, err := stmt.QueryContext(
		ctx, pq.StringArray(roomIDs), ra.Low(), ra.High(),
		pq.StringArray(senders),
		pq.StringArray(notSenders),
		pq.StringArray(filterConvertTypeWildcardToSQL(eventFilter.Types)),
		pq.StringArray(filterConvertTypeWildcardToSQL(eventFilter.NotTypes)),
		eventFilter.Limit+1,
	)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectRecentEvents: rows.close() failed")

	result := make(map[string]types.RecentEvents)

	for rows.Next() {
		var (
			roomID            string
			eventID           string
			streamPos         types.StreamPosition
			eventBytes        []byte
			excludeFromSync   bool
			sessionID         *int64
			txnID             *string
			transactionID     *api.TransactionID
			historyVisibility gomatrixserverlib.HistoryVisibility
		)
		if err := rows.Scan(&roomID, &eventID, &streamPos, &eventBytes, &sessionID, &excludeFromSync, &txnID, &historyVisibility); err != nil {
			return nil, err
		}
		// TODO: Handle redacted events
		var ev rstypes.HeaderedEvent
		if err := json.Unmarshal(eventBytes, &ev); err != nil {
			return nil, err
		}

		if sessionID != nil && txnID != nil {
			transactionID = &api.TransactionID{
				SessionID:     *sessionID,
				TransactionID: *txnID,
			}
		}

		r := result[roomID]

		ev.Visibility = historyVisibility
		r.Events = append(r.Events, types.StreamEvent{
			HeaderedEvent:   &ev,
			StreamPosition:  streamPos,
			TransactionID:   transactionID,
			ExcludeFromSync: excludeFromSync,
		})

		result[roomID] = r
	}

	if chronologicalOrder {
		for roomID, evs := range result {
			// The events need to be returned from oldest to latest, which isn't
			// necessary the way the SQL query returns them, so a sort is necessary to
			// ensure the events are in the right order in the slice.
			sort.SliceStable(evs.Events, func(i int, j int) bool {
				return evs.Events[i].StreamPosition < evs.Events[j].StreamPosition
			})

			if len(evs.Events) > eventFilter.Limit {
				evs.Limited = true
				evs.Events = evs.Events[1:]
			}

			result[roomID] = evs
		}
	} else {
		for roomID, evs := range result {
			if len(evs.Events) > eventFilter.Limit {
				evs.Limited = true
				evs.Events = evs.Events[:len(evs.Events)-1]
			}

			result[roomID] = evs
		}
	}
	return result, rows.Err()
}

// selectEvents returns the events for the given event IDs. If an event is
// missing from the database, it will be omitted.
func (s *outputRoomEventsStatements) SelectEvents(
	ctx context.Context, txn *sql.Tx, eventIDs []string, filter *synctypes.RoomEventFilter, preserveOrder bool,
) ([]types.StreamEvent, error) {
	var (
		stmt *sql.Stmt
		rows *sql.Rows
		err  error
	)
	if filter == nil {
		stmt = sqlutil.TxStmt(txn, s.selectEventsStmt)
		rows, err = stmt.QueryContext(ctx, pq.StringArray(eventIDs))
	} else {
		senders, notSenders := getSendersRoomEventFilter(filter)
		stmt = sqlutil.TxStmt(txn, s.selectEventsWitFilterStmt)
		rows, err = stmt.QueryContext(ctx,
			pq.StringArray(eventIDs),
			pq.StringArray(senders),
			pq.StringArray(notSenders),
			pq.StringArray(filterConvertTypeWildcardToSQL(filter.Types)),
			pq.StringArray(filterConvertTypeWildcardToSQL(filter.NotTypes)),
			filter.ContainsURL,
			filter.Limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectEvents: rows.close() failed")
	streamEvents, err := rowsToStreamEvents(rows)
	if err != nil {
		return nil, err
	}
	if preserveOrder {
		eventMap := make(map[string]types.StreamEvent)
		for _, ev := range streamEvents {
			eventMap[ev.EventID()] = ev
		}
		var returnEvents []types.StreamEvent
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

func (s *outputRoomEventsStatements) SelectContextEvent(ctx context.Context, txn *sql.Tx, roomID, eventID string) (id int, evt rstypes.HeaderedEvent, err error) {
	row := sqlutil.TxStmt(txn, s.selectContextEventStmt).QueryRowContext(ctx, roomID, eventID)

	var eventAsString string
	var historyVisibility gomatrixserverlib.HistoryVisibility
	if err = row.Scan(&id, &eventAsString, &historyVisibility); err != nil {
		return 0, evt, err
	}

	if err = json.Unmarshal([]byte(eventAsString), &evt); err != nil {
		return 0, evt, err
	}
	evt.Visibility = historyVisibility
	return id, evt, nil
}

func (s *outputRoomEventsStatements) SelectContextBeforeEvent(
	ctx context.Context, txn *sql.Tx, id int, roomID string, filter *synctypes.RoomEventFilter,
) (evts []*rstypes.HeaderedEvent, err error) {
	senders, notSenders := getSendersRoomEventFilter(filter)
	rows, err := sqlutil.TxStmt(txn, s.selectContextBeforeEventStmt).QueryContext(
		ctx, roomID, id, filter.Limit,
		pq.StringArray(senders),
		pq.StringArray(notSenders),
		pq.StringArray(filterConvertTypeWildcardToSQL(filter.Types)),
		pq.StringArray(filterConvertTypeWildcardToSQL(filter.NotTypes)),
	)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "rows.close() failed")

	for rows.Next() {
		var (
			eventBytes        []byte
			evt               *rstypes.HeaderedEvent
			historyVisibility gomatrixserverlib.HistoryVisibility
		)
		if err = rows.Scan(&eventBytes, &historyVisibility); err != nil {
			return evts, err
		}
		if err = json.Unmarshal(eventBytes, &evt); err != nil {
			return evts, err
		}
		evt.Visibility = historyVisibility
		evts = append(evts, evt)
	}

	return evts, rows.Err()
}

func (s *outputRoomEventsStatements) SelectContextAfterEvent(
	ctx context.Context, txn *sql.Tx, id int, roomID string, filter *synctypes.RoomEventFilter,
) (lastID int, evts []*rstypes.HeaderedEvent, err error) {
	senders, notSenders := getSendersRoomEventFilter(filter)
	rows, err := sqlutil.TxStmt(txn, s.selectContextAfterEventStmt).QueryContext(
		ctx, roomID, id, filter.Limit,
		pq.StringArray(senders),
		pq.StringArray(notSenders),
		pq.StringArray(filterConvertTypeWildcardToSQL(filter.Types)),
		pq.StringArray(filterConvertTypeWildcardToSQL(filter.NotTypes)),
	)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "rows.close() failed")

	for rows.Next() {
		var (
			eventBytes        []byte
			evt               *rstypes.HeaderedEvent
			historyVisibility gomatrixserverlib.HistoryVisibility
		)
		if err = rows.Scan(&lastID, &eventBytes, &historyVisibility); err != nil {
			return 0, evts, err
		}
		if err = json.Unmarshal(eventBytes, &evt); err != nil {
			return 0, evts, err
		}
		evt.Visibility = historyVisibility
		evts = append(evts, evt)
	}

	return lastID, evts, rows.Err()
}

func rowsToStreamEvents(rows *sql.Rows) ([]types.StreamEvent, error) {
	var result []types.StreamEvent
	for rows.Next() {
		var (
			eventID           string
			streamPos         types.StreamPosition
			eventBytes        []byte
			excludeFromSync   bool
			sessionID         *int64
			txnID             *string
			transactionID     *api.TransactionID
			historyVisibility gomatrixserverlib.HistoryVisibility
		)
		if err := rows.Scan(&eventID, &streamPos, &eventBytes, &sessionID, &excludeFromSync, &txnID, &historyVisibility); err != nil {
			return nil, err
		}
		// TODO: Handle redacted events
		var ev rstypes.HeaderedEvent
		if err := json.Unmarshal(eventBytes, &ev); err != nil {
			return nil, err
		}

		if sessionID != nil && txnID != nil {
			transactionID = &api.TransactionID{
				SessionID:     *sessionID,
				TransactionID: *txnID,
			}
		}
		ev.Visibility = historyVisibility
		result = append(result, types.StreamEvent{
			HeaderedEvent:   &ev,
			StreamPosition:  streamPos,
			TransactionID:   transactionID,
			ExcludeFromSync: excludeFromSync,
		})
	}
	return result, rows.Err()
}

func (s *outputRoomEventsStatements) PurgeEvents(
	ctx context.Context, txn *sql.Tx, roomID string,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeEventsStmt).ExecContext(ctx, roomID)
	return err
}

func (s *outputRoomEventsStatements) ReIndex(ctx context.Context, txn *sql.Tx, limit, afterID int64, types []string) (map[int64]rstypes.HeaderedEvent, error) {
	rows, err := sqlutil.TxStmt(txn, s.selectSearchStmt).QueryContext(ctx, afterID, pq.StringArray(types), limit)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "rows.close() failed")

	var eventID string
	var id int64
	result := make(map[int64]rstypes.HeaderedEvent)
	for rows.Next() {
		var ev rstypes.HeaderedEvent
		var eventBytes []byte
		if err = rows.Scan(&id, &eventID, &eventBytes); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(eventBytes, &ev); err != nil {
			return nil, err
		}
		result[id] = ev
	}
	return result, rows.Err()
}
