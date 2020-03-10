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
	"strings"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

const currentRoomStateSchema = `
-- Stores the current room state for every room.
CREATE TABLE IF NOT EXISTS syncapi_current_room_state (
    room_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    type TEXT NOT NULL,
    sender TEXT NOT NULL,
    contains_url BOOL NOT NULL DEFAULT false,
    state_key TEXT NOT NULL,
    event_json TEXT NOT NULL,
    membership TEXT,
	added_at BIGINT,
	room_version TEXT NOT NULL,
    UNIQUE (room_id, type, state_key)
);
-- for event deletion
CREATE UNIQUE INDEX IF NOT EXISTS syncapi_event_id_idx ON syncapi_current_room_state(event_id, room_id, type, sender, contains_url);
-- for querying membership states of users
-- CREATE INDEX IF NOT EXISTS syncapi_membership_idx ON syncapi_current_room_state(type, state_key, membership) WHERE membership IS NOT NULL AND membership != 'leave';
`

const upsertRoomStateSQL = "" +
	"INSERT INTO syncapi_current_room_state (room_id, event_id, type, sender, contains_url, state_key, event_json, membership, added_at)" +
	" VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)" +
	" ON CONFLICT (event_id, room_id, type, sender, contains_url)" +
	" DO UPDATE SET event_id = $2, sender=$4, contains_url=$5, event_json = $7, membership = $8, added_at = $9"

const deleteRoomStateByEventIDSQL = "" +
	"DELETE FROM syncapi_current_room_state WHERE event_id = $1"

const selectRoomIDsWithMembershipSQL = "" +
	"SELECT room_id FROM syncapi_current_room_state WHERE type = 'm.room.member' AND state_key = $1 AND membership = $2"

const selectCurrentStateSQL = "" +
	"SELECT event_json FROM syncapi_current_room_state WHERE room_id = $1" +
	" AND ( $2 IS NULL OR     sender IN ($2)  )" +
	" AND ( $3 IS NULL OR NOT(sender IN ($3)) )" +
	" AND ( $4 IS NULL OR     type   IN ($4)  )" +
	" AND ( $5 IS NULL OR NOT(type   IN ($5)) )" +
	" AND ( $6 IS NULL OR     contains_url = $6  )" +
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
	" FROM syncapi_current_room_state WHERE event_id IN ($1)"

type currentRoomStateStatements struct {
	roomVersions                    *roomVersionStatements
	streamIDStatements              *streamIDStatements
	upsertRoomStateStmt             *sql.Stmt
	deleteRoomStateByEventIDStmt    *sql.Stmt
	selectRoomIDsWithMembershipStmt *sql.Stmt
	selectCurrentStateStmt          *sql.Stmt
	selectJoinedUsersStmt           *sql.Stmt
	selectStateEventStmt            *sql.Stmt
}

func (s *currentRoomStateStatements) prepare(db *sql.DB, rvs *roomVersionStatements, streamID *streamIDStatements) (err error) {
	s.roomVersions = rvs
	s.streamIDStatements = streamID
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
	defer rows.Close() // nolint: errcheck

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
	return result, nil
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
	defer rows.Close() // nolint: errcheck

	var result []string
	for rows.Next() {
		var roomID string
		if err := rows.Scan(&roomID); err != nil {
			return nil, err
		}
		result = append(result, roomID)
	}
	return result, nil
}

// CurrentState returns all the current state events for the given room.
func (s *currentRoomStateStatements) selectCurrentState(
	ctx context.Context, txn *sql.Tx, roomID string,
	stateFilterPart *gomatrixserverlib.StateFilter,
) ([]gomatrixserverlib.Event, error) {
	stmt := common.TxStmt(txn, s.selectCurrentStateStmt)
	rows, err := stmt.QueryContext(ctx, roomID,
		nil, // FIXME: pq.StringArray(stateFilterPart.Senders),
		nil, // FIXME: pq.StringArray(stateFilterPart.NotSenders),
		nil, // FIXME: pq.StringArray(filterConvertTypeWildcardToSQL(stateFilterPart.Types)),
		nil, // FIXME: pq.StringArray(filterConvertTypeWildcardToSQL(stateFilterPart.NotTypes)),
		stateFilterPart.ContainsURL,
		stateFilterPart.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck

	if roomVersion, e := s.roomVersions.selectRoomVersion(ctx, txn, roomID); e == nil {
		return rowsToEvents(rows, roomVersion)
	} else {
		return nil, e
	}
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
	iEventIDs := make([]interface{}, len(eventIDs))
	for k, v := range eventIDs {
		iEventIDs[k] = v
	}
	query := strings.Replace(selectEventsWithEventIDsSQL, "($1)", common.QueryVariadic(len(iEventIDs)), 1)
	rows, err := txn.QueryContext(ctx, query, iEventIDs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	return rowsToStreamEvents(ctx, txn, s.roomVersions, rows)
}

func rowsToEvents(rows *sql.Rows, roomVersion gomatrixserverlib.RoomVersion) ([]gomatrixserverlib.Event, error) {
	result := []gomatrixserverlib.Event{}
	for rows.Next() {
		var eventBytes []byte
		if err := rows.Scan(&eventBytes); err != nil {
			return nil, err
		}
		// TODO: Handle redacted events
		ev, err := gomatrixserverlib.NewEventFromTrustedJSON(eventBytes, false, roomVersion)
		if err != nil {
			return nil, err
		}
		result = append(result, ev)
	}
	return result, nil
}

func (s *currentRoomStateStatements) selectStateEvent(
	ctx context.Context, txn *sql.Tx, roomID, evType, stateKey string,
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
	if roomVersion, e := s.roomVersions.selectRoomVersion(ctx, txn, roomID); e == nil {
		ev, err := gomatrixserverlib.NewEventFromTrustedJSON(res, false, roomVersion)
		return &ev, err
	} else {
		return nil, e
	}
}
