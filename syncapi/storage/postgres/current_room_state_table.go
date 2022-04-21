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

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

const currentRoomStateSchema = `
-- Stores the current room state for every room.
CREATE TABLE IF NOT EXISTS syncapi_current_room_state (
    -- The 'room_id' key for the state event.
    room_id TEXT NOT NULL,
    -- The state event ID
    event_id TEXT NOT NULL,
    -- The state event type e.g 'm.room.member'
    type TEXT NOT NULL,
    -- The 'sender' property of the event.
    sender TEXT NOT NULL,
	-- true if the event content contains a url key
    contains_url BOOL NOT NULL,
    -- The state_key value for this state event e.g ''
    state_key TEXT NOT NULL,
    -- The JSON for the event. Stored as TEXT because this should be valid UTF-8.
    headered_event_json TEXT NOT NULL,
    -- The 'content.membership' value if this event is an m.room.member event. For other
    -- events, this will be NULL.
    membership TEXT,
    -- The serial ID of the output_room_events table when this event became
    -- part of the current state of the room.
    added_at BIGINT,
    -- Clobber based on 3-uple of room_id, type and state_key
    CONSTRAINT syncapi_room_state_unique UNIQUE (room_id, type, state_key)
);
-- for event deletion
CREATE UNIQUE INDEX IF NOT EXISTS syncapi_event_id_idx ON syncapi_current_room_state(event_id, room_id, type, sender, contains_url);
-- for querying membership states of users
CREATE INDEX IF NOT EXISTS syncapi_membership_idx ON syncapi_current_room_state(type, state_key, membership) WHERE membership IS NOT NULL AND membership != 'leave';
-- for querying state by event IDs
CREATE UNIQUE INDEX IF NOT EXISTS syncapi_current_room_state_eventid_idx ON syncapi_current_room_state(event_id);
`

const upsertRoomStateSQL = "" +
	"INSERT INTO syncapi_current_room_state (room_id, event_id, type, sender, contains_url, state_key, headered_event_json, membership, added_at)" +
	" VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)" +
	" ON CONFLICT ON CONSTRAINT syncapi_room_state_unique" +
	" DO UPDATE SET event_id = $2, sender=$4, contains_url=$5, headered_event_json = $7, membership = $8, added_at = $9"

const deleteRoomStateByEventIDSQL = "" +
	"DELETE FROM syncapi_current_room_state WHERE event_id = $1"

const deleteRoomStateForRoomSQL = "" +
	"DELETE FROM syncapi_current_room_state WHERE room_id = $1"

const selectRoomIDsWithMembershipSQL = "" +
	"SELECT DISTINCT room_id FROM syncapi_current_room_state WHERE type = 'm.room.member' AND state_key = $1 AND membership = $2"

const selectRoomIDsWithAnyMembershipSQL = "" +
	"SELECT DISTINCT room_id, membership FROM syncapi_current_room_state WHERE type = 'm.room.member' AND state_key = $1"

const selectCurrentStateSQL = "" +
	"SELECT event_id, headered_event_json FROM syncapi_current_room_state WHERE room_id = $1" +
	" AND ( $2::text[] IS NULL OR     sender  = ANY($2)  )" +
	" AND ( $3::text[] IS NULL OR NOT(sender  = ANY($3)) )" +
	" AND ( $4::text[] IS NULL OR     type LIKE ANY($4)  )" +
	" AND ( $5::text[] IS NULL OR NOT(type LIKE ANY($5)) )" +
	" AND ( $6::bool IS NULL   OR     contains_url = $6  )" +
	" AND (event_id = ANY($7)) IS NOT TRUE" +
	" LIMIT $8"

const selectJoinedUsersSQL = "" +
	"SELECT room_id, state_key FROM syncapi_current_room_state WHERE type = 'm.room.member' AND membership = 'join'"

const selectStateEventSQL = "" +
	"SELECT headered_event_json FROM syncapi_current_room_state WHERE room_id = $1 AND type = $2 AND state_key = $3"

const selectEventsWithEventIDsSQL = "" +
	// TODO: The session_id and transaction_id blanks are here because otherwise
	// the rowsToStreamEvents expects there to be exactly six columns. We need to
	// figure out if these really need to be in the DB, and if so, we need a
	// better permanent fix for this. - neilalexander, 2 Jan 2020
	"SELECT event_id, added_at, headered_event_json, 0 AS session_id, false AS exclude_from_sync, '' AS transaction_id" +
	" FROM syncapi_current_room_state WHERE event_id = ANY($1)"

type currentRoomStateStatements struct {
	upsertRoomStateStmt                *sql.Stmt
	deleteRoomStateByEventIDStmt       *sql.Stmt
	deleteRoomStateForRoomStmt         *sql.Stmt
	selectRoomIDsWithMembershipStmt    *sql.Stmt
	selectRoomIDsWithAnyMembershipStmt *sql.Stmt
	selectCurrentStateStmt             *sql.Stmt
	selectJoinedUsersStmt              *sql.Stmt
	selectEventsWithEventIDsStmt       *sql.Stmt
	selectStateEventStmt               *sql.Stmt
}

func NewPostgresCurrentRoomStateTable(db *sql.DB) (tables.CurrentRoomState, error) {
	s := &currentRoomStateStatements{}
	_, err := db.Exec(currentRoomStateSchema)
	if err != nil {
		return nil, err
	}
	if s.upsertRoomStateStmt, err = db.Prepare(upsertRoomStateSQL); err != nil {
		return nil, err
	}
	if s.deleteRoomStateByEventIDStmt, err = db.Prepare(deleteRoomStateByEventIDSQL); err != nil {
		return nil, err
	}
	if s.deleteRoomStateForRoomStmt, err = db.Prepare(deleteRoomStateForRoomSQL); err != nil {
		return nil, err
	}
	if s.selectRoomIDsWithMembershipStmt, err = db.Prepare(selectRoomIDsWithMembershipSQL); err != nil {
		return nil, err
	}
	if s.selectRoomIDsWithAnyMembershipStmt, err = db.Prepare(selectRoomIDsWithAnyMembershipSQL); err != nil {
		return nil, err
	}
	if s.selectCurrentStateStmt, err = db.Prepare(selectCurrentStateSQL); err != nil {
		return nil, err
	}
	if s.selectJoinedUsersStmt, err = db.Prepare(selectJoinedUsersSQL); err != nil {
		return nil, err
	}
	if s.selectEventsWithEventIDsStmt, err = db.Prepare(selectEventsWithEventIDsSQL); err != nil {
		return nil, err
	}
	if s.selectStateEventStmt, err = db.Prepare(selectStateEventSQL); err != nil {
		return nil, err
	}
	return s, nil
}

// SelectJoinedUsers returns a map of room ID to a list of joined user IDs.
func (s *currentRoomStateStatements) SelectJoinedUsers(
	ctx context.Context,
) (map[string][]string, error) {
	rows, err := s.selectJoinedUsersStmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectJoinedUsers: rows.close() failed")

	result := make(map[string][]string)
	for rows.Next() {
		var roomID string
		var userID string
		if err := rows.Scan(&roomID, &userID); err != nil {
			return nil, err
		}
		users := result[roomID]
		users = append(users, userID)
		result[roomID] = users
	}
	return result, rows.Err()
}

// SelectRoomIDsWithMembership returns the list of room IDs which have the given user in the given membership state.
func (s *currentRoomStateStatements) SelectRoomIDsWithMembership(
	ctx context.Context,
	txn *sql.Tx,
	userID string,
	membership string, // nolint: unparam
) ([]string, error) {
	stmt := sqlutil.TxStmt(txn, s.selectRoomIDsWithMembershipStmt)
	rows, err := stmt.QueryContext(ctx, userID, membership)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectRoomIDsWithMembership: rows.close() failed")

	var result []string
	for rows.Next() {
		var roomID string
		if err := rows.Scan(&roomID); err != nil {
			return nil, err
		}
		result = append(result, roomID)
	}
	return result, rows.Err()
}

// SelectRoomIDsWithAnyMembership returns a map of all memberships for the given user.
func (s *currentRoomStateStatements) SelectRoomIDsWithAnyMembership(
	ctx context.Context,
	txn *sql.Tx,
	userID string,
) (map[string]string, error) {
	stmt := sqlutil.TxStmt(txn, s.selectRoomIDsWithAnyMembershipStmt)
	rows, err := stmt.QueryContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectRoomIDsWithAnyMembership: rows.close() failed")

	result := map[string]string{}
	for rows.Next() {
		var roomID string
		var membership string
		if err := rows.Scan(&roomID, &membership); err != nil {
			return nil, err
		}
		result[roomID] = membership
	}
	return result, rows.Err()
}

// SelectCurrentState returns all the current state events for the given room.
func (s *currentRoomStateStatements) SelectCurrentState(
	ctx context.Context, txn *sql.Tx, roomID string,
	stateFilter *gomatrixserverlib.StateFilter,
	excludeEventIDs []string,
) ([]*gomatrixserverlib.HeaderedEvent, error) {
	stmt := sqlutil.TxStmt(txn, s.selectCurrentStateStmt)
	senders, notSenders := getSendersStateFilterFilter(stateFilter)
	rows, err := stmt.QueryContext(ctx, roomID,
		pq.StringArray(senders),
		pq.StringArray(notSenders),
		pq.StringArray(filterConvertTypeWildcardToSQL(stateFilter.Types)),
		pq.StringArray(filterConvertTypeWildcardToSQL(stateFilter.NotTypes)),
		stateFilter.ContainsURL,
		pq.StringArray(excludeEventIDs),
		stateFilter.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectCurrentState: rows.close() failed")

	return rowsToEvents(rows)
}

func (s *currentRoomStateStatements) DeleteRoomStateByEventID(
	ctx context.Context, txn *sql.Tx, eventID string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteRoomStateByEventIDStmt)
	_, err := stmt.ExecContext(ctx, eventID)
	return err
}

func (s *currentRoomStateStatements) DeleteRoomStateForRoom(
	ctx context.Context, txn *sql.Tx, roomID string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteRoomStateForRoomStmt)
	_, err := stmt.ExecContext(ctx, roomID)
	return err
}

func (s *currentRoomStateStatements) UpsertRoomState(
	ctx context.Context, txn *sql.Tx,
	event *gomatrixserverlib.HeaderedEvent, membership *string, addedAt types.StreamPosition,
) error {
	// Parse content as JSON and search for an "url" key
	containsURL := false
	var content map[string]interface{}
	if json.Unmarshal(event.Content(), &content) != nil {
		// Set containsURL to true if url is present
		_, containsURL = content["url"]
	}

	headeredJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// upsert state event
	stmt := sqlutil.TxStmt(txn, s.upsertRoomStateStmt)
	_, err = stmt.ExecContext(
		ctx,
		event.RoomID(),
		event.EventID(),
		event.Type(),
		event.Sender(),
		containsURL,
		*event.StateKey(),
		headeredJSON,
		membership,
		addedAt,
	)
	return err
}

func (s *currentRoomStateStatements) SelectEventsWithEventIDs(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) ([]types.StreamEvent, error) {
	stmt := sqlutil.TxStmt(txn, s.selectEventsWithEventIDsStmt)
	rows, err := stmt.QueryContext(ctx, pq.StringArray(eventIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectEventsWithEventIDs: rows.close() failed")
	return rowsToStreamEvents(rows)
}

func rowsToEvents(rows *sql.Rows) ([]*gomatrixserverlib.HeaderedEvent, error) {
	result := []*gomatrixserverlib.HeaderedEvent{}
	for rows.Next() {
		var eventID string
		var eventBytes []byte
		if err := rows.Scan(&eventID, &eventBytes); err != nil {
			return nil, err
		}
		// TODO: Handle redacted events
		var ev gomatrixserverlib.HeaderedEvent
		if err := ev.UnmarshalJSONWithEventID(eventBytes, eventID); err != nil {
			return nil, err
		}
		result = append(result, &ev)
	}
	return result, rows.Err()
}

func (s *currentRoomStateStatements) SelectStateEvent(
	ctx context.Context, roomID, evType, stateKey string,
) (*gomatrixserverlib.HeaderedEvent, error) {
	stmt := s.selectStateEventStmt
	var res []byte
	err := stmt.QueryRowContext(ctx, roomID, evType, stateKey).Scan(&res)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ev gomatrixserverlib.HeaderedEvent
	if err = json.Unmarshal(res, &ev); err != nil {
		return nil, err
	}
	return &ev, err
}
