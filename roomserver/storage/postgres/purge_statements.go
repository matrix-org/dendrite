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
	_, err := sqlutil.TxStmt(txn, s.purgeStateBlockEntriesStmt).ExecContext(ctx, roomNID)
	return err
}

func (s *purgeStatements) PurgeStateSnapshots(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeStateSnapshotEntriesStmt).ExecContext(ctx, roomNID)
	return err
}
