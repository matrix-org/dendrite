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
	"errors"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	rstypes "github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/syncapi/storage/postgres/deltas"
	"github.com/element-hq/dendrite/syncapi/storage/tables"
	"github.com/element-hq/dendrite/syncapi/synctypes"
	"github.com/element-hq/dendrite/syncapi/types"
	"github.com/lib/pq"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
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
    history_visibility SMALLINT NOT NULL DEFAULT 2,
    -- Clobber based on 3-uple of room_id, type and state_key
    CONSTRAINT syncapi_room_state_unique UNIQUE (room_id, type, state_key)
);
-- for event deletion
CREATE UNIQUE INDEX IF NOT EXISTS syncapi_event_id_idx ON syncapi_current_room_state(event_id, room_id, type, sender, contains_url);
-- for querying membership states of users
CREATE INDEX IF NOT EXISTS syncapi_membership_idx ON syncapi_current_room_state(type, state_key, membership) WHERE membership IS NOT NULL AND membership != 'leave';
-- for querying state by event IDs
CREATE UNIQUE INDEX IF NOT EXISTS syncapi_current_room_state_eventid_idx ON syncapi_current_room_state(event_id);
-- for improving selectRoomIDsWithAnyMembershipSQL
CREATE INDEX IF NOT EXISTS syncapi_current_room_state_type_state_key_idx ON syncapi_current_room_state(type, state_key);
`

const upsertRoomStateSQL = "" +
	"INSERT INTO syncapi_current_room_state (room_id, event_id, type, sender, contains_url, state_key, headered_event_json, membership, added_at, history_visibility)" +
	" VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)" +
	" ON CONFLICT ON CONSTRAINT syncapi_room_state_unique" +
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
	"SELECT event_id, headered_event_json FROM syncapi_current_room_state WHERE room_id = $1" +
	" AND ( $2::text[] IS NULL OR     sender  = ANY($2)  )" +
	" AND ( $3::text[] IS NULL OR NOT(sender  = ANY($3)) )" +
	" AND ( $4::text[] IS NULL OR     type LIKE ANY($4)  )" +
	" AND ( $5::text[] IS NULL OR NOT(type LIKE ANY($5)) )" +
	" AND ( $6::bool IS NULL   OR     contains_url = $6  )" +
	" AND (event_id = ANY($7)) IS NOT TRUE"

const selectJoinedUsersSQL = "" +
	"SELECT room_id, state_key FROM syncapi_current_room_state WHERE type = 'm.room.member' AND membership = 'join'"

const selectJoinedUsersInRoomSQL = "" +
	"SELECT room_id, state_key FROM syncapi_current_room_state WHERE type = 'm.room.member' AND membership = 'join' AND room_id = ANY($1)"

const selectStateEventSQL = "" +
	"SELECT headered_event_json FROM syncapi_current_room_state WHERE room_id = $1 AND type = $2 AND state_key = $3"

const selectEventsWithEventIDsSQL = "" +
	"SELECT event_id, added_at, headered_event_json, history_visibility FROM syncapi_current_room_state WHERE event_id = ANY($1)"

const selectSharedUsersSQL = "" +
	"SELECT state_key FROM syncapi_current_room_state WHERE room_id = ANY(" +
	"	SELECT DISTINCT room_id FROM syncapi_current_room_state WHERE state_key = $1 AND membership='join'" +
	") AND type = 'm.room.member' AND state_key = ANY($2) AND membership IN ('join', 'invite');"

const selectMembershipCount = `SELECT count(*) FROM syncapi_current_room_state WHERE type = 'm.room.member' AND room_id = $1 AND membership = $2`

const selectRoomHeroes = `
SELECT state_key FROM syncapi_current_room_state
WHERE type = 'm.room.member' AND room_id = $1 AND membership = ANY($2) AND state_key != $3
ORDER BY added_at, state_key
LIMIT 5
`

type currentRoomStateStatements struct {
	upsertRoomStateStmt                *sql.Stmt
	deleteRoomStateByEventIDStmt       *sql.Stmt
	deleteRoomStateForRoomStmt         *sql.Stmt
	selectRoomIDsWithMembershipStmt    *sql.Stmt
	selectRoomIDsWithAnyMembershipStmt *sql.Stmt
	selectCurrentStateStmt             *sql.Stmt
	selectJoinedUsersStmt              *sql.Stmt
	selectJoinedUsersInRoomStmt        *sql.Stmt
	selectEventsWithEventIDsStmt       *sql.Stmt
	selectStateEventStmt               *sql.Stmt
	selectSharedUsersStmt              *sql.Stmt
	selectMembershipCountStmt          *sql.Stmt
	selectRoomHeroesStmt               *sql.Stmt
}

func NewPostgresCurrentRoomStateTable(db *sql.DB) (tables.CurrentRoomState, error) {
	s := &currentRoomStateStatements{}
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

	return s, sqlutil.StatementList{
		{&s.upsertRoomStateStmt, upsertRoomStateSQL},
		{&s.deleteRoomStateByEventIDStmt, deleteRoomStateByEventIDSQL},
		{&s.deleteRoomStateForRoomStmt, deleteRoomStateForRoomSQL},
		{&s.selectRoomIDsWithMembershipStmt, selectRoomIDsWithMembershipSQL},
		{&s.selectRoomIDsWithAnyMembershipStmt, selectRoomIDsWithAnyMembershipSQL},
		{&s.selectCurrentStateStmt, selectCurrentStateSQL},
		{&s.selectJoinedUsersStmt, selectJoinedUsersSQL},
		{&s.selectJoinedUsersInRoomStmt, selectJoinedUsersInRoomSQL},
		{&s.selectEventsWithEventIDsStmt, selectEventsWithEventIDsSQL},
		{&s.selectStateEventStmt, selectStateEventSQL},
		{&s.selectSharedUsersStmt, selectSharedUsersSQL},
		{&s.selectMembershipCountStmt, selectMembershipCount},
		{&s.selectRoomHeroesStmt, selectRoomHeroes},
	}.Prepare(db)
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
	return result, rows.Err()
}

// SelectJoinedUsersInRoom returns a map of room ID to a list of joined user IDs for a given room.
func (s *currentRoomStateStatements) SelectJoinedUsersInRoom(
	ctx context.Context, txn *sql.Tx, roomIDs []string,
) (map[string][]string, error) {
	rows, err := sqlutil.TxStmt(txn, s.selectJoinedUsersInRoomStmt).QueryContext(ctx, pq.StringArray(roomIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectJoinedUsers: rows.close() failed")

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
	stateFilter *synctypes.StateFilter,
	excludeEventIDs []string,
) ([]*rstypes.HeaderedEvent, error) {
	stmt := sqlutil.TxStmt(txn, s.selectCurrentStateStmt)
	senders, notSenders := getSendersStateFilterFilter(stateFilter)
	// We're going to query members later, so remove them from this request
	if stateFilter.LazyLoadMembers && !stateFilter.IncludeRedundantMembers {
		notTypes := &[]string{spec.MRoomMember}
		if stateFilter.NotTypes != nil {
			*stateFilter.NotTypes = append(*stateFilter.NotTypes, spec.MRoomMember)
		} else {
			stateFilter.NotTypes = notTypes
		}
	}
	rows, err := stmt.QueryContext(ctx, roomID,
		pq.StringArray(senders),
		pq.StringArray(notSenders),
		pq.StringArray(filterConvertTypeWildcardToSQL(stateFilter.Types)),
		pq.StringArray(filterConvertTypeWildcardToSQL(stateFilter.NotTypes)),
		stateFilter.ContainsURL,
		pq.StringArray(excludeEventIDs),
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
	event *rstypes.HeaderedEvent, membership *string, addedAt types.StreamPosition,
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
		event.RoomID().String(),
		event.EventID(),
		event.Type(),
		event.UserID.String(),
		containsURL,
		*event.StateKeyResolved,
		headeredJSON,
		membership,
		addedAt,
		event.Visibility,
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
	return currentRoomStateRowsToStreamEvents(rows)
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
		var ev rstypes.HeaderedEvent
		if err := json.Unmarshal(eventBytes, &ev); err != nil {
			return nil, err
		}

		ev.Visibility = historyVisibility

		events = append(events, types.StreamEvent{
			HeaderedEvent:  &ev,
			StreamPosition: streamPos,
		})
	}

	return events, rows.Err()
}

func rowsToEvents(rows *sql.Rows) ([]*rstypes.HeaderedEvent, error) {
	result := []*rstypes.HeaderedEvent{}
	for rows.Next() {
		var eventID string
		var eventBytes []byte
		if err := rows.Scan(&eventID, &eventBytes); err != nil {
			return nil, err
		}
		// TODO: Handle redacted events
		var ev rstypes.HeaderedEvent
		if err := json.Unmarshal(eventBytes, &ev); err != nil {
			return nil, err
		}
		result = append(result, &ev)
	}
	return result, rows.Err()
}

func (s *currentRoomStateStatements) SelectStateEvent(
	ctx context.Context, txn *sql.Tx, roomID, evType, stateKey string,
) (*rstypes.HeaderedEvent, error) {
	stmt := sqlutil.TxStmt(txn, s.selectStateEventStmt)
	var res []byte
	err := stmt.QueryRowContext(ctx, roomID, evType, stateKey).Scan(&res)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ev rstypes.HeaderedEvent
	if err = json.Unmarshal(res, &ev); err != nil {
		return nil, err
	}
	return &ev, err
}

func (s *currentRoomStateStatements) SelectSharedUsers(
	ctx context.Context, txn *sql.Tx, userID string, otherUserIDs []string,
) ([]string, error) {
	stmt := sqlutil.TxStmt(txn, s.selectSharedUsersStmt)
	rows, err := stmt.QueryContext(ctx, userID, pq.Array(otherUserIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectSharedUsersStmt: rows.close() failed")

	var stateKey string
	result := make([]string, 0, len(otherUserIDs))
	for rows.Next() {
		if err := rows.Scan(&stateKey); err != nil {
			return nil, err
		}
		result = append(result, stateKey)
	}
	return result, rows.Err()
}

func (s *currentRoomStateStatements) SelectRoomHeroes(ctx context.Context, txn *sql.Tx, roomID, excludeUserID string, memberships []string) ([]string, error) {
	stmt := sqlutil.TxStmt(txn, s.selectRoomHeroesStmt)
	rows, err := stmt.QueryContext(ctx, roomID, pq.StringArray(memberships), excludeUserID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectRoomHeroesStmt: rows.close() failed")

	var stateKey string
	result := make([]string, 0, 5)
	for rows.Next() {
		if err = rows.Scan(&stateKey); err != nil {
			return nil, err
		}
		result = append(result, stateKey)
	}
	return result, rows.Err()
}

func (s *currentRoomStateStatements) SelectMembershipCount(ctx context.Context, txn *sql.Tx, roomID, membership string) (count int, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectMembershipCountStmt)
	err = stmt.QueryRowContext(ctx, roomID, membership).Scan(&count)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}
