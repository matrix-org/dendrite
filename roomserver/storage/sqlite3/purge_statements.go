// Copyright 2022 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/types"
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
		{&s.purgeRedactionStmt, purgeRedactionsSQL},
		{&s.purgeRoomAliasesStmt, purgeRoomAliasesSQL},
		{&s.purgeRoomStmt, purgeRoomSQL},
		//{&s.purgeStateBlockEntriesStmt, purgeStateBlockEntriesSQL},
		{&s.purgeStateSnapshotEntriesStmt, purgeStateSnapshotEntriesSQL},
	}.Prepare(db)
}

func (s *purgeStatements) PurgeEventJSONs(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeEventJSONStmt).ExecContext(ctx, roomNID)
	return err
}

func (s *purgeStatements) PurgeEvents(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeEventsStmt).ExecContext(ctx, roomNID)
	return err
}

func (s *purgeStatements) PurgeInvites(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeInvitesStmt).ExecContext(ctx, roomNID)
	return err
}

func (s *purgeStatements) PurgeMemberships(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeMembershipsStmt).ExecContext(ctx, roomNID)
	return err
}

func (s *purgeStatements) PurgePreviousEvents(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgePreviousEventsStmt).ExecContext(ctx, roomNID)
	return err
}

func (s *purgeStatements) PurgePublished(
	ctx context.Context, txn *sql.Tx, roomID string,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgePublishedStmt).ExecContext(ctx, roomID)
	return err
}

func (s *purgeStatements) PurgeRedactions(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeRedactionStmt).ExecContext(ctx, roomNID)
	return err
}

func (s *purgeStatements) PurgeRoomAliases(
	ctx context.Context, txn *sql.Tx, roomID string,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeRoomAliasesStmt).ExecContext(ctx, roomID)
	return err
}

func (s *purgeStatements) PurgeRoom(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeRoomStmt).ExecContext(ctx, roomNID)
	return err
}

func (s *purgeStatements) PurgeStateBlocks(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) error {
	// Get all stateBlockNIDs
	stateBlockNIDs, err := s.stateSnapshot.selectStateBlockNIDsForRoomNID(ctx, txn, roomNID)
	if err != nil {
		return err
	}

	params := make([]interface{}, len(stateBlockNIDs)+1)
	for k, v := range stateBlockNIDs {
		params[k] = v
	}

	query := "DELETE FROM roomserver_state_block WHERE state_block_nid IN($1)"
	return sqlutil.RunLimitedVariablesExec(ctx, query, txn, params, sqlutil.SQLite3MaxVariables)
}

func (s *purgeStatements) PurgeStateSnapshots(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeStateSnapshotEntriesStmt).ExecContext(ctx, roomNID)
	return err
}
