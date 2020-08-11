// Copyright 2020 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/currentstateserver/storage/tables"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

const currentRoomStateSchema = `
-- Stores the current room state for every room.
CREATE TABLE IF NOT EXISTS currentstate_current_room_state (
    room_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    type TEXT NOT NULL,
    sender TEXT NOT NULL,
    state_key TEXT NOT NULL,
    headered_event_json TEXT NOT NULL,
    content_value TEXT NOT NULL DEFAULT '',
    UNIQUE (room_id, type, state_key)
);
-- for event deletion
CREATE UNIQUE INDEX IF NOT EXISTS currentstate_event_id_idx ON currentstate_current_room_state(event_id, room_id, type, sender);
`

const upsertRoomStateSQL = "" +
	"INSERT INTO currentstate_current_room_state (room_id, event_id, type, sender, state_key, headered_event_json, content_value)" +
	" VALUES ($1, $2, $3, $4, $5, $6, $7)" +
	" ON CONFLICT (event_id, room_id, type, sender)" +
	" DO UPDATE SET event_id = $2, sender=$4, headered_event_json = $6, content_value = $7"

const deleteRoomStateByEventIDSQL = "" +
	"DELETE FROM currentstate_current_room_state WHERE event_id = $1"

const selectRoomIDsWithMembershipSQL = "" +
	"SELECT room_id FROM currentstate_current_room_state WHERE type = 'm.room.member' AND state_key = $1 AND content_value = $2"

const selectStateEventSQL = "" +
	"SELECT headered_event_json FROM currentstate_current_room_state WHERE room_id = $1 AND type = $2 AND state_key = $3"

const selectEventsWithEventIDsSQL = "" +
	"SELECT headered_event_json FROM currentstate_current_room_state WHERE event_id IN ($1)"

const selectBulkStateContentSQL = "" +
	"SELECT room_id, type, state_key, content_value FROM currentstate_current_room_state WHERE room_id IN ($1) AND type IN ($2) AND state_key IN ($3)"

const selectBulkStateContentWildSQL = "" +
	"SELECT room_id, type, state_key, content_value FROM currentstate_current_room_state WHERE room_id IN ($1) AND type IN ($2)"

const selectJoinedUsersSetForRoomsSQL = "" +
	"SELECT state_key, COUNT(room_id) FROM currentstate_current_room_state WHERE room_id IN ($1) AND type = 'm.room.member' and content_value = 'join' GROUP BY state_key"

const selectKnownRoomsSQL = "" +
	"SELECT DISTINCT room_id FROM currentstate_current_room_state"

// selectKnownUsersSQL uses a sub-select statement here to find rooms that the user is
// joined to. Since this information is used to populate the user directory, we will
// only return users that the user would ordinarily be able to see anyway.
const selectKnownUsersSQL = "" +
	"SELECT DISTINCT state_key FROM currentstate_current_room_state WHERE room_id IN (" +
	"  SELECT DISTINCT room_id FROM currentstate_current_room_state WHERE state_key=$1 AND TYPE='m.room.member' AND content_value='join'" +
	") AND TYPE='m.room.member' AND content_value='join' AND state_key LIKE $2 LIMIT $3"

type currentRoomStateStatements struct {
	db                               *sql.DB
	writer                           *sqlutil.TransactionWriter
	upsertRoomStateStmt              *sql.Stmt
	deleteRoomStateByEventIDStmt     *sql.Stmt
	selectRoomIDsWithMembershipStmt  *sql.Stmt
	selectStateEventStmt             *sql.Stmt
	selectJoinedUsersSetForRoomsStmt *sql.Stmt
	selectKnownRoomsStmt             *sql.Stmt
	selectKnownUsersStmt             *sql.Stmt
}

func NewSqliteCurrentRoomStateTable(db *sql.DB) (tables.CurrentRoomState, error) {
	s := &currentRoomStateStatements{
		db:     db,
		writer: sqlutil.NewTransactionWriter(),
	}
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
	if s.selectRoomIDsWithMembershipStmt, err = db.Prepare(selectRoomIDsWithMembershipSQL); err != nil {
		return nil, err
	}
	if s.selectStateEventStmt, err = db.Prepare(selectStateEventSQL); err != nil {
		return nil, err
	}
	if s.selectJoinedUsersSetForRoomsStmt, err = db.Prepare(selectJoinedUsersSetForRoomsSQL); err != nil {
		return nil, err
	}
	if s.selectKnownRoomsStmt, err = db.Prepare(selectKnownRoomsSQL); err != nil {
		return nil, err
	}
	if s.selectKnownUsersStmt, err = db.Prepare(selectKnownUsersSQL); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *currentRoomStateStatements) SelectJoinedUsersSetForRooms(ctx context.Context, roomIDs []string) (map[string]int, error) {
	iRoomIDs := make([]interface{}, len(roomIDs))
	for i, v := range roomIDs {
		iRoomIDs[i] = v
	}
	query := strings.Replace(selectJoinedUsersSetForRoomsSQL, "($1)", sqlutil.QueryVariadic(len(iRoomIDs)), 1)
	rows, err := s.db.QueryContext(ctx, query, iRoomIDs...)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectJoinedUsersSetForRooms: rows.close() failed")
	result := make(map[string]int)
	for rows.Next() {
		var userID string
		var count int
		if err := rows.Scan(&userID, &count); err != nil {
			return nil, err
		}
		result[userID] = count
	}
	return result, rows.Err()
}

// SelectRoomIDsWithMembership returns the list of room IDs which have the given user in the given membership state.
func (s *currentRoomStateStatements) SelectRoomIDsWithMembership(
	ctx context.Context,
	txn *sql.Tx,
	userID string,
	membership string,
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

func (s *currentRoomStateStatements) DeleteRoomStateByEventID(
	ctx context.Context, txn *sql.Tx, eventID string,
) error {
	return s.writer.Do(s.db, txn, func(txn *sql.Tx) error {
		stmt := sqlutil.TxStmt(txn, s.deleteRoomStateByEventIDStmt)
		_, err := stmt.ExecContext(ctx, eventID)
		return err
	})
}

func (s *currentRoomStateStatements) UpsertRoomState(
	ctx context.Context, txn *sql.Tx,
	event gomatrixserverlib.HeaderedEvent, contentVal string,
) error {
	headeredJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// upsert state event
	return s.writer.Do(s.db, txn, func(txn *sql.Tx) error {
		stmt := sqlutil.TxStmt(txn, s.upsertRoomStateStmt)
		_, err = stmt.ExecContext(
			ctx,
			event.RoomID(),
			event.EventID(),
			event.Type(),
			event.Sender(),
			*event.StateKey(),
			headeredJSON,
			contentVal,
		)
		return err
	})
}

func (s *currentRoomStateStatements) SelectEventsWithEventIDs(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) ([]gomatrixserverlib.HeaderedEvent, error) {
	iEventIDs := make([]interface{}, len(eventIDs))
	for k, v := range eventIDs {
		iEventIDs[k] = v
	}
	query := strings.Replace(selectEventsWithEventIDsSQL, "($1)", sqlutil.QueryVariadic(len(iEventIDs)), 1)
	var rows *sql.Rows
	var err error
	if txn != nil {
		rows, err = txn.QueryContext(ctx, query, iEventIDs...)
	} else {
		rows, err = s.db.QueryContext(ctx, query, iEventIDs...)
	}
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectEventsWithEventIDs: rows.close() failed")
	result := []gomatrixserverlib.HeaderedEvent{}
	for rows.Next() {
		var eventBytes []byte
		if err := rows.Scan(&eventBytes); err != nil {
			return nil, err
		}
		var ev gomatrixserverlib.HeaderedEvent
		if err := json.Unmarshal(eventBytes, &ev); err != nil {
			return nil, err
		}
		result = append(result, ev)
	}
	return result, nil
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

func (s *currentRoomStateStatements) SelectBulkStateContent(
	ctx context.Context, roomIDs []string, tuples []gomatrixserverlib.StateKeyTuple, allowWildcards bool,
) ([]tables.StrippedEvent, error) {
	hasWildcards := false
	eventTypeSet := make(map[string]bool)
	stateKeySet := make(map[string]bool)
	var eventTypes []string
	var stateKeys []string
	for _, tuple := range tuples {
		if !eventTypeSet[tuple.EventType] {
			eventTypeSet[tuple.EventType] = true
			eventTypes = append(eventTypes, tuple.EventType)
		}
		if !stateKeySet[tuple.StateKey] {
			stateKeySet[tuple.StateKey] = true
			stateKeys = append(stateKeys, tuple.StateKey)
		}
		if tuple.StateKey == "*" {
			hasWildcards = true
		}
	}

	iRoomIDs := make([]interface{}, len(roomIDs))
	for i, v := range roomIDs {
		iRoomIDs[i] = v
	}
	iEventTypes := make([]interface{}, len(eventTypes))
	for i, v := range eventTypes {
		iEventTypes[i] = v
	}
	iStateKeys := make([]interface{}, len(stateKeys))
	for i, v := range stateKeys {
		iStateKeys[i] = v
	}

	var query string
	var args []interface{}
	if hasWildcards && allowWildcards {
		query = strings.Replace(selectBulkStateContentWildSQL, "($1)", sqlutil.QueryVariadic(len(iRoomIDs)), 1)
		query = strings.Replace(query, "($2)", sqlutil.QueryVariadicOffset(len(iEventTypes), len(iRoomIDs)), 1)
		args = append(iRoomIDs, iEventTypes...)
	} else {
		query = strings.Replace(selectBulkStateContentSQL, "($1)", sqlutil.QueryVariadic(len(iRoomIDs)), 1)
		query = strings.Replace(query, "($2)", sqlutil.QueryVariadicOffset(len(iEventTypes), len(iRoomIDs)), 1)
		query = strings.Replace(query, "($3)", sqlutil.QueryVariadicOffset(len(iStateKeys), len(iEventTypes)+len(iRoomIDs)), 1)
		args = append(iRoomIDs, iEventTypes...)
		args = append(args, iStateKeys...)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)

	if err != nil {
		return nil, err
	}
	strippedEvents := []tables.StrippedEvent{}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectBulkStateContent: rows.close() failed")
	for rows.Next() {
		var roomID string
		var eventType string
		var stateKey string
		var contentVal string
		if err = rows.Scan(&roomID, &eventType, &stateKey, &contentVal); err != nil {
			return nil, err
		}
		strippedEvents = append(strippedEvents, tables.StrippedEvent{
			RoomID:       roomID,
			ContentValue: contentVal,
			EventType:    eventType,
			StateKey:     stateKey,
		})
	}
	return strippedEvents, rows.Err()
}

func (s *currentRoomStateStatements) SelectKnownUsers(ctx context.Context, userID, searchString string, limit int) ([]string, error) {
	rows, err := s.selectKnownUsersStmt.QueryContext(ctx, userID, fmt.Sprintf("%%%s%%", searchString), limit)
	if err != nil {
		return nil, err
	}
	result := []string{}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectKnownUsers: rows.close() failed")
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		result = append(result, userID)
	}
	return result, rows.Err()
}

func (s *currentRoomStateStatements) SelectKnownRooms(ctx context.Context) ([]string, error) {
	rows, err := s.selectKnownRoomsStmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	result := []string{}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectKnownRooms: rows.close() failed")
	for rows.Next() {
		var roomID string
		if err := rows.Scan(&roomID); err != nil {
			return nil, err
		}
		result = append(result, roomID)
	}
	return result, rows.Err()
}
