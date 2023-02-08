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

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
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

type inviteEventsStatements struct {
	insertInviteEventStmt         *sql.Stmt
	selectInviteEventsInRangeStmt *sql.Stmt
	deleteInviteEventStmt         *sql.Stmt
	selectMaxInviteIDStmt         *sql.Stmt
}

func NewPostgresInvitesTable(db *sql.DB) (tables.Invites, error) {
	s := &inviteEventsStatements{}
	_, err := db.Exec(inviteEventsSchema)
	if err != nil {
		return nil, err
	}
	if s.insertInviteEventStmt, err = db.Prepare(insertInviteEventSQL); err != nil {
		return nil, err
	}
	if s.selectInviteEventsInRangeStmt, err = db.Prepare(selectInviteEventsInRangeSQL); err != nil {
		return nil, err
	}
	if s.deleteInviteEventStmt, err = db.Prepare(deleteInviteEventSQL); err != nil {
		return nil, err
	}
	if s.selectMaxInviteIDStmt, err = db.Prepare(selectMaxInviteIDSQL); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *inviteEventsStatements) InsertInviteEvent(
	ctx context.Context, txn *sql.Tx, inviteEvent *gomatrixserverlib.HeaderedEvent,
) (streamPos types.StreamPosition, err error) {
	var headeredJSON []byte
	headeredJSON, err = json.Marshal(inviteEvent)
	if err != nil {
		return
	}

	err = sqlutil.TxStmt(txn, s.insertInviteEventStmt).QueryRowContext(
		ctx,
		inviteEvent.RoomID(),
		inviteEvent.EventID(),
		*inviteEvent.StateKey(),
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
) (map[string]*gomatrixserverlib.HeaderedEvent, map[string]*gomatrixserverlib.HeaderedEvent, types.StreamPosition, error) {
	var lastPos types.StreamPosition
	stmt := sqlutil.TxStmt(txn, s.selectInviteEventsInRangeStmt)
	rows, err := stmt.QueryContext(ctx, targetUserID, r.Low(), r.High())
	if err != nil {
		return nil, nil, lastPos, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectInviteEventsInRange: rows.close() failed")
	result := map[string]*gomatrixserverlib.HeaderedEvent{}
	retired := map[string]*gomatrixserverlib.HeaderedEvent{}
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

		var event *gomatrixserverlib.HeaderedEvent
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
