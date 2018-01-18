package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
)

const inviteEventsSchema = `
CREATE TABLE IF NOT EXISTS syncapi_invite_events (
	id BIGINT PRIMARY KEY DEFAULT nextval('syncapi_stream_id'),
	event_id TEXT NOT NULL,
	room_id TEXT NOT NULL,
	type TEXT NOT NULL,
	sender TEXT NOT NULL,
    contains_url BOOL NOT NULL,
	target_user_id TEXT NOT NULL,
	event_json TEXT NOT NULL
);

-- For looking up the invites for a given user.
CREATE INDEX IF NOT EXISTS syncapi_invites_target_user_id_idx
	ON syncapi_invite_events (target_user_id, id, room_id, type, sender, contains_url);

-- For deleting old invites
CREATE INDEX IF NOT EXISTS syncapi_invites_event_id_idx
	ON syncapi_invite_events(target_user_id, id);
`

const insertInviteEventSQL = "" +
	"INSERT INTO syncapi_invite_events (" +
	" room_id, event_id, type, sender, contains_url, target_user_id, event_json" +
	") VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id"

const deleteInviteEventSQL = "" +
	"DELETE FROM syncapi_invite_events WHERE event_id = $1"

const selectInviteEventsInRangeSQL = "" +
	"SELECT room_id, event_json FROM syncapi_invite_events" +
	" WHERE target_user_id = $1 AND id > $2 AND id <= $3" +
	" AND ( $4::text[] IS NULL OR     sender  = ANY($4)  )" +
	" AND ( $5::text[] IS NULL OR NOT(sender  = ANY($5)) )" +
	" AND ( $6::text[] IS NULL OR     type LIKE ANY($6)  )" +
	" AND ( $7::text[] IS NULL OR NOT(type LIKE ANY($7)) )" +
	" AND ( $8::text[] IS NULL OR     room_id = ANY($8)  )" +
	" AND ( $9::text[] IS NULL OR NOT(room_id = ANY($9)) )" +
	" AND ( $10::bool IS NULL  OR     contains_url = $10 )" +
	" ORDER BY id DESC"

const selectMaxInviteIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_invite_events"

type inviteEventsStatements struct {
	insertInviteEventStmt         *sql.Stmt
	selectInviteEventsInRangeStmt *sql.Stmt
	deleteInviteEventStmt         *sql.Stmt
	selectMaxInviteIDStmt         *sql.Stmt
}

func (s *inviteEventsStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(inviteEventsSchema)
	if err != nil {
		return
	}
	if s.insertInviteEventStmt, err = db.Prepare(insertInviteEventSQL); err != nil {
		return
	}
	if s.selectInviteEventsInRangeStmt, err = db.Prepare(selectInviteEventsInRangeSQL); err != nil {
		return
	}
	if s.deleteInviteEventStmt, err = db.Prepare(deleteInviteEventSQL); err != nil {
		return
	}
	if s.selectMaxInviteIDStmt, err = db.Prepare(selectMaxInviteIDSQL); err != nil {
		return
	}
	return
}

func (s *inviteEventsStatements) insertInviteEvent(
	ctx context.Context, inviteEvent gomatrixserverlib.Event,
) (streamPos int64, err error) {
	var content map[string]interface{}
	json.Unmarshal(inviteEvent.Content(), content)
	_, containsURL := content["url"]

	err = s.insertInviteEventStmt.QueryRowContext(
		ctx,
		inviteEvent.RoomID(),
		inviteEvent.EventID(),
		inviteEvent.Type(),
		inviteEvent.Sender(),
		containsURL,
		*inviteEvent.StateKey(),
		inviteEvent.JSON(),
	).Scan(&streamPos)
	return
}

func (s *inviteEventsStatements) deleteInviteEvent(
	ctx context.Context, inviteEventID string,
) error {
	_, err := s.deleteInviteEventStmt.ExecContext(ctx, inviteEventID)
	return err
}

// selectInviteEventsInRange returns a map of room ID to invite event for the
// active invites for the target user ID in the supplied range.
func (s *inviteEventsStatements) selectInviteEventsInRange(
	ctx context.Context, txn *sql.Tx, targetUserID string, startPos, endPos int64, filter *gomatrix.Filter,
) (map[string]gomatrixserverlib.Event, error) {
	stmt := common.TxStmt(txn, s.selectInviteEventsInRangeStmt)

	rows, err := stmt.QueryContext(ctx, targetUserID, startPos, endPos,
		pq.StringArray(filterConvertWildcardToSQL(filter.Room.State.Types)),
		pq.StringArray(filterConvertWildcardToSQL(filter.Room.State.NotTypes)),
		pq.StringArray(filter.Room.State.Senders),
		pq.StringArray(filter.Room.State.NotSenders),
		pq.StringArray(append(filter.Room.Rooms, filter.Room.State.Rooms...)),
		pq.StringArray(append(filter.Room.NotRooms, filter.Room.State.NotRooms...)),
		filter.Room.State.ContainsURL,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	result := map[string]gomatrixserverlib.Event{}
	for rows.Next() {
		var (
			roomID    string
			eventJSON []byte
		)
		if err = rows.Scan(&roomID, &eventJSON); err != nil {
			return nil, err
		}

		event, err := gomatrixserverlib.NewEventFromTrustedJSON(eventJSON, false)
		if err != nil {
			return nil, err
		}

		result[roomID] = event
	}
	return result, nil
}

func (s *inviteEventsStatements) selectMaxInviteID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64
	stmt := common.TxStmt(txn, s.selectMaxInviteIDStmt)
	err = stmt.QueryRowContext(ctx).Scan(&nullableID)
	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}
