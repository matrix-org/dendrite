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

package postgres

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const purgeEventJSONSQL = "" +
	"DELETE FROM roomserver_event_json WHERE event_nid = ANY(" +
	"	SELECT event_nid FROM roomserver_events WHERE room_nid = $1" +
	")"

const purgeEventsSQL = "" +
	"DELETE FROM roomserver_events WHERE room_nid = $1"

const purgeInvitesSQL = "" +
	"DELETE FROM roomserver_invites WHERE room_nid = $1"

const purgeMembershipsSQL = "" +
	"DELETE FROM roomserver_membership WHERE room_nid = $1"

const purgePreviousEventsSQL = "" +
	"DELETE FROM roomserver_previous_events WHERE event_nids && ANY(" +
	"	SELECT ARRAY_AGG(event_nid) FROM roomserver_events WHERE room_nid = $1" +
	")"

const purgePublishedSQL = "" +
	"DELETE FROM roomserver_published WHERE room_id = $1"

const purgeRedactionsSQL = "" +
	"DELETE FROM roomserver_redactions WHERE redaction_event_id = ANY(" +
	"	SELECT event_id FROM roomserver_events WHERE room_nid = $1" +
	")"

const purgeRoomAliasesSQL = "" +
	"DELETE FROM roomserver_room_aliases WHERE room_id = $1"

const purgeRoomSQL = "" +
	"DELETE FROM roomserver_rooms WHERE room_nid = $1"

const purgeStateBlockEntriesSQL = "" +
	"DELETE FROM roomserver_state_block WHERE state_block_nid = ANY(" +
	"	SELECT DISTINCT UNNEST(state_block_nids) FROM roomserver_state_snapshots WHERE room_nid = $1" +
	")"

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
	purgeStateBlockEntriesStmt    *sql.Stmt
	purgeStateSnapshotEntriesStmt *sql.Stmt
}

func PreparePurgeStatements(db *sql.DB) (*purgeStatements, error) {
	s := &purgeStatements{}

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
		{&s.purgeStateBlockEntriesStmt, purgeStateBlockEntriesSQL},
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
	purgeByRoomNID := []*sql.Stmt{
		s.purgeStateBlockEntriesStmt,
		s.purgeStateSnapshotEntriesStmt,
		s.purgeInvitesStmt,
		s.purgeMembershipsStmt,
		s.purgePreviousEventsStmt,
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
