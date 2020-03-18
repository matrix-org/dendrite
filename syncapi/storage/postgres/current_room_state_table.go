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
	"github.com/matrix-org/dendrite/common"
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
    event_json TEXT NOT NULL,
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
`

const upsertRoomStateSQL = "" +
	"INSERT INTO syncapi_current_room_state (room_id, event_id, type, sender, contains_url, state_key, event_json, membership, added_at)" +
	" VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)" +
	" ON CONFLICT ON CONSTRAINT syncapi_room_state_unique" +
	" DO UPDATE SET event_id = $2, sender=$4, contains_url=$5, event_json = $7, membership = $8, added_at = $9"

const deleteRoomStateByEventIDSQL = "" +
	"DELETE FROM syncapi_current_room_state WHERE event_id = $1"

const selectRoomIDsWithMembershipSQL = "" +
	"SELECT room_id FROM syncapi_current_room_state WHERE type = 'm.room.member' AND state_key = $1 AND membership = $2"

const selectCurrentStateSQL = "" +
	"SELECT event_json FROM syncapi_current_room_state WHERE room_id = $1" +
	" AND ( $2::text[] IS NULL OR     sender  = ANY($2)  )" +
	" AND ( $3::text[] IS NULL OR NOT(sender  = ANY($3)) )" +
	" AND ( $4::text[] IS NULL OR     type LIKE ANY($4)  )" +
	" AND ( $5::text[] IS NULL OR NOT(type LIKE ANY($5)) )" +
	" AND ( $6::bool IS NULL   OR     contains_url = $6  )" +
	" LIMIT $7"

const selectJoinedUsersSQL = "" +
	"SELECT room_id, state_key FROM syncapi_current_room_state WHERE type = 'm.room.member' AND membership = 'join'"

const selectStateEventSQL = "" +
	"SELECT event_json FROM syncapi_current_room_state WHERE room_id = $1 AND type = $2 AND state_key = $3"

const selectEventsWithEventIDsSQL = "" +
	// TODO: The session_id and transaction_id blanks are here because otherwise
	// the rowsToStreamEvents expects there to be exactly five columns. We need to
	// figure out if these really need to be in the DB, and if so, we need a
	// better permanent fix for this. - neilalexander, 2 Jan 2020
	"SELECT added_at, event_json, 0 AS session_id, false AS exclude_from_sync, '' AS transaction_id" +
	" FROM syncapi_current_room_state WHERE event_id = ANY($1)"

type currentRoomStateStatements struct {
	upsertRoomStateStmt             *sql.Stmt
	deleteRoomStateByEventIDStmt    *sql.Stmt
	selectRoomIDsWithMembershipStmt *sql.Stmt
	selectCurrentStateStmt          *sql.Stmt
	selectJoinedUsersStmt           *sql.Stmt
	selectEventsWithEventIDsStmt    *sql.Stmt
	selectStateEventStmt            *sql.Stmt
}

func (s *currentRoomStateStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(currentRoomStateSchema)
	if err != nil {
		return
	}
	if s.upsertRoomStateStmt, err = db.Prepare(upsertRoomStateSQL); err != nil {
		return
	}
	if s.deleteRoomStateByEventIDStmt, err = db.Prepare(deleteRoomStateByEventIDSQL); err != nil {
		return
	}
	if s.selectRoomIDsWithMembershipStmt, err = db.Prepare(selectRoomIDsWithMembershipSQL); err != nil {
		return
	}
	if s.selectCurrentStateStmt, err = db.Prepare(selectCurrentStateSQL); err != nil {
		return
	}
	if s.selectJoinedUsersStmt, err = db.Prepare(selectJoinedUsersSQL); err != nil {
		return
	}
	if s.selectEventsWithEventIDsStmt, err = db.Prepare(selectEventsWithEventIDsSQL); err != nil {
		return
	}
	if s.selectStateEventStmt, err = db.Prepare(selectStateEventSQL); err != nil {
		return
	}
	return
}

// JoinedMemberLists returns a map of room ID to a list of joined user IDs.
func (s *currentRoomStateStatements) selectJoinedUsers(
	ctx context.Context,
) (map[string][]string, error) {
	rows, err := s.selectJoinedUsersStmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer common.CloseAndLogIfError(ctx, rows, "selectJoinedUsers: rows.close() failed")

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
func (s *currentRoomStateStatements) selectRoomIDsWithMembership(
	ctx context.Context,
	txn *sql.Tx,
	userID string,
	membership string, // nolint: unparam
) ([]string, error) {
	stmt := common.TxStmt(txn, s.selectRoomIDsWithMembershipStmt)
	rows, err := stmt.QueryContext(ctx, userID, membership)
	if err != nil {
		return nil, err
	}
	defer common.CloseAndLogIfError(ctx, rows, "selectRoomIDsWithMembership: rows.close() failed")

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

// CurrentState returns all the current state events for the given room.
func (s *currentRoomStateStatements) selectCurrentState(
	ctx context.Context, txn *sql.Tx, roomID string,
	stateFilter *gomatrixserverlib.StateFilter,
) ([]gomatrixserverlib.Event, error) {
	stmt := common.TxStmt(txn, s.selectCurrentStateStmt)
	rows, err := stmt.QueryContext(ctx, roomID,
		pq.StringArray(stateFilter.Senders),
		pq.StringArray(stateFilter.NotSenders),
		pq.StringArray(filterConvertTypeWildcardToSQL(stateFilter.Types)),
		pq.StringArray(filterConvertTypeWildcardToSQL(stateFilter.NotTypes)),
		stateFilter.ContainsURL,
		stateFilter.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer common.CloseAndLogIfError(ctx, rows, "selectCurrentState: rows.close() failed")

	return rowsToEvents(rows)
}

func (s *currentRoomStateStatements) deleteRoomStateByEventID(
	ctx context.Context, txn *sql.Tx, eventID string,
) error {
	stmt := common.TxStmt(txn, s.deleteRoomStateByEventIDStmt)
	_, err := stmt.ExecContext(ctx, eventID)
	return err
}

func (s *currentRoomStateStatements) upsertRoomState(
	ctx context.Context, txn *sql.Tx,
	event gomatrixserverlib.Event, membership *string, addedAt types.StreamPosition,
) error {
	// Parse content as JSON and search for an "url" key
	containsURL := false
	var content map[string]interface{}
	if json.Unmarshal(event.Content(), &content) != nil {
		// Set containsURL to true if url is present
		_, containsURL = content["url"]
	}

	// upsert state event
	stmt := common.TxStmt(txn, s.upsertRoomStateStmt)
	_, err := stmt.ExecContext(
		ctx,
		event.RoomID(),
		event.EventID(),
		event.Type(),
		event.Sender(),
		containsURL,
		*event.StateKey(),
		event.JSON(),
		membership,
		addedAt,
	)
	return err
}

func (s *currentRoomStateStatements) selectEventsWithEventIDs(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) ([]types.StreamEvent, error) {
	stmt := common.TxStmt(txn, s.selectEventsWithEventIDsStmt)
	rows, err := stmt.QueryContext(ctx, pq.StringArray(eventIDs))
	if err != nil {
		return nil, err
	}
	defer common.CloseAndLogIfError(ctx, rows, "selectEventsWithEventIDs: rows.close() failed")
	return rowsToStreamEvents(rows)
}

func rowsToEvents(rows *sql.Rows) ([]gomatrixserverlib.Event, error) {
	result := []gomatrixserverlib.Event{}
	for rows.Next() {
		var eventBytes []byte
		if err := rows.Scan(&eventBytes); err != nil {
			return nil, err
		}
		// TODO: Handle redacted events
		ev, err := gomatrixserverlib.NewEventFromTrustedJSON(eventBytes, false)
		if err != nil {
			return nil, err
		}
		result = append(result, ev)
	}
	return result, rows.Err()
}

func (s *currentRoomStateStatements) selectStateEvent(
	ctx context.Context, roomID, evType, stateKey string,
) (*gomatrixserverlib.Event, error) {
	stmt := s.selectStateEventStmt
	var res []byte
	err := stmt.QueryRowContext(ctx, roomID, evType, stateKey).Scan(&res)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ev, err := gomatrixserverlib.NewEventFromTrustedJSON(res, false)
	return &ev, err
}
