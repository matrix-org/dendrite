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

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/federationapi/types"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

const joinedHostsSchema = `
-- The joined_hosts table stores a list of m.room.member event ids in the
-- current state for each room where the membership is "join".
-- There will be an entry for every user that is joined to the room.
CREATE TABLE IF NOT EXISTS federationsender_joined_hosts (
    -- The string ID of the room.
    room_id TEXT NOT NULL,
    -- The event ID of the m.room.member join event.
    event_id TEXT NOT NULL,
    -- The domain part of the user ID the m.room.member event is for.
    server_name TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS federatonsender_joined_hosts_event_id_idx
    ON federationsender_joined_hosts (event_id);

CREATE INDEX IF NOT EXISTS federatonsender_joined_hosts_room_id_idx
    ON federationsender_joined_hosts (room_id)
`

const insertJoinedHostsSQL = "" +
	"INSERT INTO federationsender_joined_hosts (room_id, event_id, server_name)" +
	" VALUES ($1, $2, $3) ON CONFLICT DO NOTHING"

const deleteJoinedHostsSQL = "" +
	"DELETE FROM federationsender_joined_hosts WHERE event_id = ANY($1)"

const deleteJoinedHostsForRoomSQL = "" +
	"DELETE FROM federationsender_joined_hosts WHERE room_id = $1"

const selectJoinedHostsSQL = "" +
	"SELECT event_id, server_name FROM federationsender_joined_hosts" +
	" WHERE room_id = $1"

const selectAllJoinedHostsSQL = "" +
	"SELECT DISTINCT server_name FROM federationsender_joined_hosts"

const selectJoinedHostsForRoomsSQL = "" +
	"SELECT DISTINCT server_name FROM federationsender_joined_hosts WHERE room_id = ANY($1)"

const selectJoinedHostsForRoomsExcludingBlacklistedSQL = "" +
	"SELECT DISTINCT server_name FROM federationsender_joined_hosts j WHERE room_id = ANY($1) AND NOT EXISTS (" +
	"  SELECT server_name FROM federationsender_blacklist WHERE j.server_name = server_name" +
	");"

type joinedHostsStatements struct {
	db                                                *sql.DB
	insertJoinedHostsStmt                             *sql.Stmt
	deleteJoinedHostsStmt                             *sql.Stmt
	deleteJoinedHostsForRoomStmt                      *sql.Stmt
	selectJoinedHostsStmt                             *sql.Stmt
	selectAllJoinedHostsStmt                          *sql.Stmt
	selectJoinedHostsForRoomsStmt                     *sql.Stmt
	selectJoinedHostsForRoomsExcludingBlacklistedStmt *sql.Stmt
}

func NewPostgresJoinedHostsTable(db *sql.DB) (s *joinedHostsStatements, err error) {
	s = &joinedHostsStatements{
		db: db,
	}
	_, err = s.db.Exec(joinedHostsSchema)
	if err != nil {
		return
	}
	if s.insertJoinedHostsStmt, err = s.db.Prepare(insertJoinedHostsSQL); err != nil {
		return
	}
	if s.deleteJoinedHostsStmt, err = s.db.Prepare(deleteJoinedHostsSQL); err != nil {
		return
	}
	if s.deleteJoinedHostsForRoomStmt, err = s.db.Prepare(deleteJoinedHostsForRoomSQL); err != nil {
		return
	}
	if s.selectJoinedHostsStmt, err = s.db.Prepare(selectJoinedHostsSQL); err != nil {
		return
	}
	if s.selectAllJoinedHostsStmt, err = s.db.Prepare(selectAllJoinedHostsSQL); err != nil {
		return
	}
	if s.selectJoinedHostsForRoomsStmt, err = s.db.Prepare(selectJoinedHostsForRoomsSQL); err != nil {
		return
	}
	if s.selectJoinedHostsForRoomsExcludingBlacklistedStmt, err = s.db.Prepare(selectJoinedHostsForRoomsExcludingBlacklistedSQL); err != nil {
		return
	}
	return
}

func (s *joinedHostsStatements) InsertJoinedHosts(
	ctx context.Context,
	txn *sql.Tx,
	roomID, eventID string,
	serverName gomatrixserverlib.ServerName,
) error {
	stmt := sqlutil.TxStmt(txn, s.insertJoinedHostsStmt)
	_, err := stmt.ExecContext(ctx, roomID, eventID, serverName)
	return err
}

func (s *joinedHostsStatements) DeleteJoinedHosts(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteJoinedHostsStmt)
	_, err := stmt.ExecContext(ctx, pq.StringArray(eventIDs))
	return err
}

func (s *joinedHostsStatements) DeleteJoinedHostsForRoom(
	ctx context.Context, txn *sql.Tx, roomID string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteJoinedHostsForRoomStmt)
	_, err := stmt.ExecContext(ctx, roomID)
	return err
}

func (s *joinedHostsStatements) SelectJoinedHostsWithTx(
	ctx context.Context, txn *sql.Tx, roomID string,
) ([]types.JoinedHost, error) {
	stmt := sqlutil.TxStmt(txn, s.selectJoinedHostsStmt)
	return joinedHostsFromStmt(ctx, stmt, roomID)
}

func (s *joinedHostsStatements) SelectJoinedHosts(
	ctx context.Context, roomID string,
) ([]types.JoinedHost, error) {
	return joinedHostsFromStmt(ctx, s.selectJoinedHostsStmt, roomID)
}

func (s *joinedHostsStatements) SelectAllJoinedHosts(
	ctx context.Context,
) ([]gomatrixserverlib.ServerName, error) {
	rows, err := s.selectAllJoinedHostsStmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectAllJoinedHosts: rows.close() failed")

	var result []gomatrixserverlib.ServerName
	for rows.Next() {
		var serverName string
		if err = rows.Scan(&serverName); err != nil {
			return nil, err
		}
		result = append(result, gomatrixserverlib.ServerName(serverName))
	}

	return result, rows.Err()
}

func (s *joinedHostsStatements) SelectJoinedHostsForRooms(
	ctx context.Context, roomIDs []string, excludingBlacklisted bool,
) ([]gomatrixserverlib.ServerName, error) {
	stmt := s.selectJoinedHostsForRoomsStmt
	if excludingBlacklisted {
		stmt = s.selectJoinedHostsForRoomsExcludingBlacklistedStmt
	}
	rows, err := stmt.QueryContext(ctx, pq.StringArray(roomIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectJoinedHostsForRoomsStmt: rows.close() failed")

	var result []gomatrixserverlib.ServerName
	for rows.Next() {
		var serverName string
		if err = rows.Scan(&serverName); err != nil {
			return nil, err
		}
		result = append(result, gomatrixserverlib.ServerName(serverName))
	}

	return result, rows.Err()
}

func joinedHostsFromStmt(
	ctx context.Context, stmt *sql.Stmt, roomID string,
) ([]types.JoinedHost, error) {
	rows, err := stmt.QueryContext(ctx, roomID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "joinedHostsFromStmt: rows.close() failed")

	var result []types.JoinedHost
	for rows.Next() {
		var eventID, serverName string
		if err = rows.Scan(&eventID, &serverName); err != nil {
			return nil, err
		}
		result = append(result, types.JoinedHost{
			MemberEventID: eventID,
			ServerName:    gomatrixserverlib.ServerName(serverName),
		})
	}

	return result, rows.Err()
}
