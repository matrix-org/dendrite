// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package sqlite3

import (
	"context"
	"database/sql"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/roomserver/types"
)

const purgeEventJSONSQL = "" +
	"DELETE FROM roomserver_event_json WHERE event_nid IN (" +
	"	SELECT event_nid FROM roomserver_events WHERE room_nid = $1" +
	")"

const purgeEventsSQL = "" +
	"DELETE FROM roomserver_events WHERE room_nid = $1"

const purgeInvitesSQL = "" +
	"DELETE FROM roomserver_invites WHERE room_nid = $1"

const purgeMembershipsSQL = "" +
	"DELETE FROM roomserver_membership WHERE room_nid = $1"

const purgePreviousEventsSQL = "" +
	"DELETE FROM roomserver_previous_events WHERE event_nids IN(" +
	"	SELECT event_nid FROM roomserver_events WHERE room_nid = $1" +
	")"

// This removes the majority of prev events and is way faster than the above.
// The above query is still needed to delete the remaining prev events.
const purgePreviousEvents2SQL = "" +
	"DELETE FROM roomserver_previous_events AS rpe WHERE EXISTS(SELECT event_id FROM roomserver_events AS re WHERE room_nid = $1 AND re.event_id = rpe.previous_event_id)"

const purgePublishedSQL = "" +
	"DELETE FROM roomserver_published WHERE room_id = $1"

const purgeRedactionsSQL = "" +
	"DELETE FROM roomserver_redactions WHERE redaction_event_id IN(" +
	"	SELECT event_id FROM roomserver_events WHERE room_nid = $1" +
	")"

const purgeRoomAliasesSQL = "" +
	"DELETE FROM roomserver_room_aliases WHERE room_id = $1"

const purgeRoomSQL = "" +
	"DELETE FROM roomserver_rooms WHERE room_nid = $1"

const purgeStateSnapshotEntriesSQL = "" +
	"DELETE FROM roomserver_state_snapshots WHERE room_nid = $1"

type purgeStatements struct {
	purgeEventJSONStmt            *sql.Stmt
	purgeEventsStmt               *sql.Stmt
	purgeInvitesStmt              *sql.Stmt
	purgeMembershipsStmt          *sql.Stmt
	purgePreviousEventsStmt       *sql.Stmt
	purgePreviousEvents2Stmt      *sql.Stmt
	purgePublishedStmt            *sql.Stmt
	purgeRedactionStmt            *sql.Stmt
	purgeRoomAliasesStmt          *sql.Stmt
	purgeRoomStmt                 *sql.Stmt
	purgeStateSnapshotEntriesStmt *sql.Stmt
	stateSnapshot                 *stateSnapshotStatements
}

func PreparePurgeStatements(db *sql.DB, stateSnapshot *stateSnapshotStatements) (*purgeStatements, error) {
	s := &purgeStatements{stateSnapshot: stateSnapshot}
	return s, sqlutil.StatementList{
		{&s.purgeEventJSONStmt, purgeEventJSONSQL},
		{&s.purgeEventsStmt, purgeEventsSQL},
		{&s.purgeInvitesStmt, purgeInvitesSQL},
		{&s.purgeMembershipsStmt, purgeMembershipsSQL},
		{&s.purgePublishedStmt, purgePublishedSQL},
		{&s.purgePreviousEventsStmt, purgePreviousEventsSQL},
		{&s.purgePreviousEvents2Stmt, purgePreviousEvents2SQL},
		{&s.purgeRedactionStmt, purgeRedactionsSQL},
		{&s.purgeRoomAliasesStmt, purgeRoomAliasesSQL},
		{&s.purgeRoomStmt, purgeRoomSQL},
		//{&s.purgeStateBlockEntriesStmt, purgeStateBlockEntriesSQL},
		{&s.purgeStateSnapshotEntriesStmt, purgeStateSnapshotEntriesSQL},
	}.Prepare(db)
}

func (s *purgeStatements) PurgeRoom(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, roomID string,
) error {

	// purge by roomID
	purgeByRoomID := []*sql.Stmt{
		s.purgeRoomAliasesStmt,
		s.purgePublishedStmt,
	}
	for _, stmt := range purgeByRoomID {
		_, err := sqlutil.TxStmt(txn, stmt).ExecContext(ctx, roomID)
		if err != nil {
			return err
		}
	}

	// purge by roomNID
	if err := s.purgeStateBlocks(ctx, txn, roomNID); err != nil {
		return err
	}

	purgeByRoomNID := []*sql.Stmt{
		s.purgeStateSnapshotEntriesStmt,
		s.purgeInvitesStmt,
		s.purgeMembershipsStmt,
		s.purgePreviousEvents2Stmt, // Fast purge the majority of events
		s.purgePreviousEventsStmt,  // Slow purge the remaining events
		s.purgeEventJSONStmt,
		s.purgeRedactionStmt,
		s.purgeEventsStmt,
		s.purgeRoomStmt,
	}
	for _, stmt := range purgeByRoomNID {
		_, err := sqlutil.TxStmt(txn, stmt).ExecContext(ctx, roomNID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *purgeStatements) purgeStateBlocks(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) error {
	// Get all stateBlockNIDs
	stateBlockNIDs, err := s.stateSnapshot.selectStateBlockNIDsForRoomNID(ctx, txn, roomNID)
	if err != nil {
		return err
	}
	params := make([]interface{}, len(stateBlockNIDs))
	seenNIDs := make(map[types.StateBlockNID]struct{}, len(stateBlockNIDs))
	// dedupe NIDs
	for k, v := range stateBlockNIDs {
		if _, ok := seenNIDs[v]; ok {
			continue
		}
		params[k] = v
		seenNIDs[v] = struct{}{}
	}

	query := "DELETE FROM roomserver_state_block WHERE state_block_nid IN($1)"
	return sqlutil.RunLimitedVariablesExec(ctx, query, txn, params, sqlutil.SQLite3MaxVariables)
}
