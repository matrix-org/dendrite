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

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	rstypes "github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/syncapi/storage/tables"
	"github.com/element-hq/dendrite/syncapi/types"
)

const inviteEventsSchema = `
CREATE TABLE IF NOT EXISTS syncapi_invite_events (
	id BIGINT PRIMARY KEY DEFAULT nextval('syncapi_stream_id'),
	event_id TEXT NOT NULL,
	room_id TEXT NOT NULL,
	target_user_id TEXT NOT NULL,
	headered_event_json TEXT NOT NULL,
	deleted BOOL NOT NULL
);

-- For looking up the invites for a given user.
CREATE INDEX IF NOT EXISTS syncapi_invites_target_user_id_idx
	ON syncapi_invite_events (target_user_id, id);

-- For deleting old invites
CREATE INDEX IF NOT EXISTS syncapi_invites_event_id_idx
	ON syncapi_invite_events (event_id);
`

const insertInviteEventSQL = "" +
	"INSERT INTO syncapi_invite_events (" +
	" room_id, event_id, target_user_id, headered_event_json, deleted" +
	") VALUES ($1, $2, $3, $4, FALSE) RETURNING id"

const deleteInviteEventSQL = "" +
	"UPDATE syncapi_invite_events SET deleted=TRUE, id=nextval('syncapi_stream_id') WHERE event_id = $1 AND deleted=FALSE RETURNING id"

const selectInviteEventsInRangeSQL = "" +
	"SELECT id, room_id, headered_event_json, deleted FROM syncapi_invite_events" +
	" WHERE target_user_id = $1 AND id > $2 AND id <= $3" +
	" ORDER BY id DESC"

const selectMaxInviteIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_invite_events"

const purgeInvitesSQL = "" +
	"DELETE FROM syncapi_invite_events WHERE room_id = $1"

type inviteEventsStatements struct {
	insertInviteEventStmt         *sql.Stmt
	selectInviteEventsInRangeStmt *sql.Stmt
	deleteInviteEventStmt         *sql.Stmt
	selectMaxInviteIDStmt         *sql.Stmt
	purgeInvitesStmt              *sql.Stmt
}

func NewPostgresInvitesTable(db *sql.DB) (tables.Invites, error) {
	s := &inviteEventsStatements{}
	_, err := db.Exec(inviteEventsSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.insertInviteEventStmt, insertInviteEventSQL},
		{&s.selectInviteEventsInRangeStmt, selectInviteEventsInRangeSQL},
		{&s.deleteInviteEventStmt, deleteInviteEventSQL},
		{&s.selectMaxInviteIDStmt, selectMaxInviteIDSQL},
		{&s.purgeInvitesStmt, purgeInvitesSQL},
	}.Prepare(db)
}

func (s *inviteEventsStatements) InsertInviteEvent(
	ctx context.Context, txn *sql.Tx, inviteEvent *rstypes.HeaderedEvent,
) (streamPos types.StreamPosition, err error) {
	var headeredJSON []byte
	headeredJSON, err = json.Marshal(inviteEvent)
	if err != nil {
		return
	}

	err = sqlutil.TxStmt(txn, s.insertInviteEventStmt).QueryRowContext(
		ctx,
		inviteEvent.RoomID().String(),
		inviteEvent.EventID(),
		inviteEvent.UserID.String(),
		headeredJSON,
	).Scan(&streamPos)
	return
}

func (s *inviteEventsStatements) DeleteInviteEvent(
	ctx context.Context, txn *sql.Tx, inviteEventID string,
) (sp types.StreamPosition, err error) {
	stmt := sqlutil.TxStmt(txn, s.deleteInviteEventStmt)
	err = stmt.QueryRowContext(ctx, inviteEventID).Scan(&sp)
	return
}

// selectInviteEventsInRange returns a map of room ID to invite event for the
// active invites for the target user ID in the supplied range.
func (s *inviteEventsStatements) SelectInviteEventsInRange(
	ctx context.Context, txn *sql.Tx, targetUserID string, r types.Range,
) (map[string]*rstypes.HeaderedEvent, map[string]*rstypes.HeaderedEvent, types.StreamPosition, error) {
	var lastPos types.StreamPosition
	stmt := sqlutil.TxStmt(txn, s.selectInviteEventsInRangeStmt)
	rows, err := stmt.QueryContext(ctx, targetUserID, r.Low(), r.High())
	if err != nil {
		return nil, nil, lastPos, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectInviteEventsInRange: rows.close() failed")
	result := map[string]*rstypes.HeaderedEvent{}
	retired := map[string]*rstypes.HeaderedEvent{}
	for rows.Next() {
		var (
			id        types.StreamPosition
			roomID    string
			eventJSON []byte
			deleted   bool
		)
		if err = rows.Scan(&id, &roomID, &eventJSON, &deleted); err != nil {
			return nil, nil, lastPos, err
		}
		if id > lastPos {
			lastPos = id
		}

		// if we have seen this room before, it has a higher stream position and hence takes priority
		// because the query is ORDER BY id DESC so drop them
		_, isRetired := retired[roomID]
		_, isInvited := result[roomID]
		if isRetired || isInvited {
			continue
		}

		var event *rstypes.HeaderedEvent
		if err := json.Unmarshal(eventJSON, &event); err != nil {
			return nil, nil, lastPos, err
		}

		if deleted {
			retired[roomID] = event
		} else {
			result[roomID] = event
		}
	}
	if lastPos == 0 {
		lastPos = r.To
	}
	return result, retired, lastPos, rows.Err()
}

func (s *inviteEventsStatements) SelectMaxInviteID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64
	stmt := sqlutil.TxStmt(txn, s.selectMaxInviteIDStmt)
	err = stmt.QueryRowContext(ctx).Scan(&nullableID)
	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}

func (s *inviteEventsStatements) PurgeInvites(
	ctx context.Context, txn *sql.Tx, roomID string,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeInvitesStmt).ExecContext(ctx, roomID)
	return err
}
