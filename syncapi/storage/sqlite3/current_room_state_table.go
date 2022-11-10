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
	"strings"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/sqlite3/deltas"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
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
    headered_event_json TEXT NOT NULL,
    membership TEXT,
    added_at BIGINT,
    history_visibility SMALLINT NOT NULL DEFAULT 2, -- The history visibility before this event (1 - world_readable; 2 - shared; 3 - invited; 4 - joined)
    UNIQUE (room_id, type, state_key)
);
-- for event deletion
CREATE UNIQUE INDEX IF NOT EXISTS syncapi_event_id_idx ON syncapi_current_room_state(event_id, room_id, type, sender, contains_url);
-- for querying membership states of users
-- CREATE INDEX IF NOT EXISTS syncapi_membership_idx ON syncapi_current_room_state(type, state_key, membership) WHERE membership IS NOT NULL AND membership != 'leave';
-- for querying state by event IDs
CREATE UNIQUE INDEX IF NOT EXISTS syncapi_current_room_state_eventid_idx ON syncapi_current_room_state(event_id);
-- for improving selectRoomIDsWithAnyMembershipSQL
CREATE INDEX IF NOT EXISTS syncapi_current_room_state_type_state_key_idx ON syncapi_current_room_state(type, state_key);
`

const upsertRoomStateSQL = "" +
	"INSERT INTO syncapi_current_room_state (room_id, event_id, type, sender, contains_url, state_key, headered_event_json, membership, added_at, history_visibility)" +
	" VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)" +
	" ON CONFLICT (room_id, type, state_key)" +
	" DO UPDATE SET event_id = $2, sender=$4, contains_url=$5, headered_event_json = $7, membership = $8, added_at = $9"

const deleteRoomStateByEventIDSQL = "" +
	"DELETE FROM syncapi_current_room_state WHERE event_id = $1"

const deleteRoomStateForRoomSQL = "" +
	"DELETE FROM syncapi_current_room_state WHERE room_id = $1"

const selectRoomIDsWithMembershipSQL = "" +
	"SELECT DISTINCT room_id FROM syncapi_current_room_state WHERE type = 'm.room.member' AND state_key = $1 AND membership = $2"

const selectRoomIDsWithAnyMembershipSQL = "" +
	"SELECT room_id, membership FROM syncapi_current_room_state WHERE type = 'm.room.member' AND state_key = $1"

const selectCurrentStateSQL = "" +
	"SELECT event_id, headered_event_json FROM syncapi_current_room_state WHERE room_id = $1"

// WHEN, ORDER BY and LIMIT will be added by prepareWithFilter

const selectJoinedUsersSQL = "" +
	"SELECT room_id, state_key FROM syncapi_current_room_state WHERE type = 'm.room.member' AND membership = 'join'"

const selectJoinedUsersInRoomSQL = "" +
	"SELECT room_id, state_key FROM syncapi_current_room_state WHERE type = 'm.room.member' AND membership = 'join' AND room_id IN ($1)"

const selectStateEventSQL = "" +
	"SELECT headered_event_json FROM syncapi_current_room_state WHERE room_id = $1 AND type = $2 AND state_key = $3"

const selectEventsWithEventIDsSQL = "" +
	"SELECT event_id, added_at, headered_event_json, history_visibility FROM syncapi_current_room_state WHERE event_id IN ($1)"

const selectSharedUsersSQL = "" +
	"SELECT state_key FROM syncapi_current_room_state WHERE room_id IN(" +
	"	SELECT DISTINCT room_id FROM syncapi_current_room_state WHERE state_key = $1 AND membership='join'" +
	") AND type = 'm.room.member' AND state_key IN ($2) AND membership IN ('join', 'invite');"

type currentRoomStateStatements struct {
	db                                 *sql.DB
	streamIDStatements                 *StreamIDStatements
	upsertRoomStateStmt                *sql.Stmt
	deleteRoomStateByEventIDStmt       *sql.Stmt
	deleteRoomStateForRoomStmt         *sql.Stmt
	selectRoomIDsWithMembershipStmt    *sql.Stmt
	selectRoomIDsWithAnyMembershipStmt *sql.Stmt
	selectJoinedUsersStmt              *sql.Stmt
	//selectJoinedUsersInRoomStmt      *sql.Stmt - prepared at runtime due to variadic
	selectStateEventStmt *sql.Stmt
	//selectSharedUsersSQL             *sql.Stmt - prepared at runtime due to variadic
}

func NewSqliteCurrentRoomStateTable(db *sql.DB, streamID *StreamIDStatements) (tables.CurrentRoomState, error) {
	s := &currentRoomStateStatements{
		db:                 db,
		streamIDStatements: streamID,
	}
	_, err := db.Exec(currentRoomStateSchema)
	if err != nil {
		return nil, err
	}

	m := sqlutil.NewMigrator(db)
	m.AddMigrations(sqlutil.Migration{
		Version: "syncapi: add history visibility column (current_room_state)",
		Up:      deltas.UpAddHistoryVisibilityColumnCurrentRoomState,
	})
	err = m.Up(context.Background())
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
	if s.selectJoinedUsersStmt, err = db.Prepare(selectJoinedUsersSQL); err != nil {
		return nil, err
	}
	//if s.selectJoinedUsersInRoomStmt, err = db.Prepare(selectJoinedUsersInRoomSQL); err != nil {
	//	return nil, err
	//}
	if s.selectStateEventStmt, err = db.Prepare(selectStateEventSQL); err != nil {
		return nil, err
	}
	return s, nil
}

// SelectJoinedUsers returns a map of room ID to a list of joined user IDs.
func (s *currentRoomStateStatements) SelectJoinedUsers(
	ctx context.Context, txn *sql.Tx,
) (map[string][]string, error) {
	rows, err := sqlutil.TxStmt(txn, s.selectJoinedUsersStmt).QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectJoinedUsers: rows.close() failed")

	result := make(map[string][]string)
	var roomID string
	var userID string
	for rows.Next() {
		if err := rows.Scan(&roomID, &userID); err != nil {
			return nil, err
		}
		users := result[roomID]
		users = append(users, userID)
		result[roomID] = users
	}
	return result, nil
}

// SelectJoinedUsersInRoom returns a map of room ID to a list of joined user IDs for a given room.
func (s *currentRoomStateStatements) SelectJoinedUsersInRoom(
	ctx context.Context, txn *sql.Tx, roomIDs []string,
) (map[string][]string, error) {
	query := strings.Replace(selectJoinedUsersInRoomSQL, "($1)", sqlutil.QueryVariadic(len(roomIDs)), 1)
	params := make([]interface{}, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		params = append(params, roomID)
	}
	stmt, err := s.db.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("SelectJoinedUsersInRoom s.db.Prepare: %w", err)
	}
	defer internal.CloseAndLogIfError(ctx, stmt, "SelectJoinedUsersInRoom: stmt.close() failed")

	rows, err := sqlutil.TxStmt(txn, stmt).QueryContext(ctx, params...)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectJoinedUsersInRoom: rows.close() failed")

	result := make(map[string][]string)
	var userID, roomID string
	for rows.Next() {
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
	return result, nil
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

// CurrentState returns all the current state events for the given room.
func (s *currentRoomStateStatements) SelectCurrentState(
	ctx context.Context, txn *sql.Tx, roomID string,
	stateFilter *gomatrixserverlib.StateFilter,
	excludeEventIDs []string,
) ([]*gomatrixserverlib.HeaderedEvent, error) {
	stmt, params, err := prepareWithFilters(
		s.db, txn, selectCurrentStateSQL,
		[]interface{}{
			roomID,
		},
		stateFilter.Senders, stateFilter.NotSenders,
		stateFilter.Types, stateFilter.NotTypes,
		excludeEventIDs, stateFilter.ContainsURL, 0,
		FilterOrderNone,
	)
	if err != nil {
		return nil, fmt.Errorf("s.prepareWithFilters: %w", err)
	}

	rows, err := stmt.QueryContext(ctx, params...)
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
		event.Visibility,
	)
	return err
}

func minOfInts(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

func (s *currentRoomStateStatements) SelectEventsWithEventIDs(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) ([]types.StreamEvent, error) {
	iEventIDs := make([]interface{}, len(eventIDs))
	for k, v := range eventIDs {
		iEventIDs[k] = v
	}
	res := make([]types.StreamEvent, 0, len(eventIDs))
	var start int
	for start < len(eventIDs) {
		n := minOfInts(len(eventIDs)-start, 999)
		query := strings.Replace(selectEventsWithEventIDsSQL, "($1)", sqlutil.QueryVariadic(n), 1)
		var rows *sql.Rows
		var err error
		if txn == nil {
			rows, err = s.db.QueryContext(ctx, query, iEventIDs[start:start+n]...)
		} else {
			rows, err = txn.QueryContext(ctx, query, iEventIDs[start:start+n]...)
		}
		if err != nil {
			return nil, err
		}
		start = start + n
		events, err := currentRoomStateRowsToStreamEvents(rows)
		internal.CloseAndLogIfError(ctx, rows, "selectEventsWithEventIDs: rows.close() failed")
		if err != nil {
			return nil, err
		}
		res = append(res, events...)
	}
	return res, nil
}

func currentRoomStateRowsToStreamEvents(rows *sql.Rows) ([]types.StreamEvent, error) {
	var events []types.StreamEvent
	for rows.Next() {
		var (
			eventID           string
			streamPos         types.StreamPosition
			eventBytes        []byte
			historyVisibility gomatrixserverlib.HistoryVisibility
		)
		if err := rows.Scan(&eventID, &streamPos, &eventBytes, &historyVisibility); err != nil {
			return nil, err
		}
		// TODO: Handle redacted events
		var ev gomatrixserverlib.HeaderedEvent
		if err := ev.UnmarshalJSONWithEventID(eventBytes, eventID); err != nil {
			return nil, err
		}

		ev.Visibility = historyVisibility

		events = append(events, types.StreamEvent{
			HeaderedEvent:  &ev,
			StreamPosition: streamPos,
		})
	}

	return events, nil
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
	return result, nil
}

func (s *currentRoomStateStatements) SelectStateEvent(
	ctx context.Context, txn *sql.Tx, roomID, evType, stateKey string,
) (*gomatrixserverlib.HeaderedEvent, error) {
	stmt := sqlutil.TxStmt(txn, s.selectStateEventStmt)
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

func (s *currentRoomStateStatements) SelectSharedUsers(
	ctx context.Context, txn *sql.Tx, userID string, otherUserIDs []string,
) ([]string, error) {

	params := make([]interface{}, len(otherUserIDs)+1)
	params[0] = userID
	for k, v := range otherUserIDs {
		params[k+1] = v
	}

	var provider sqlutil.QueryProvider
	if txn == nil {
		provider = s.db
	} else {
		provider = txn
	}

	result := make([]string, 0, len(otherUserIDs))
	query := strings.Replace(selectSharedUsersSQL, "($2)", sqlutil.QueryVariadicOffset(len(otherUserIDs), 1), 1)
	err := sqlutil.RunLimitedVariablesQuery(
		ctx, query, provider, params, sqlutil.SQLite3MaxVariables,
		func(rows *sql.Rows) error {
			var stateKey string
			for rows.Next() {
				if err := rows.Scan(&stateKey); err != nil {
					return err
				}
				result = append(result, stateKey)
			}
			return nil
		},
	)

	return result, err
}
