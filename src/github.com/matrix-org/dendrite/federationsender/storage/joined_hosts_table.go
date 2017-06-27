// Copyright 2017 Vector Creations Ltd
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

package storage

import (
	"database/sql"
	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/gomatrixserverlib"
)

const joinedHostsSchema = `
-- The joined_hosts table stores a list of m.room.member event ids in the
-- current state for each room where the membership is "join".
CREATE TABLE IF NOT EXISTS joined_hosts (
    -- The string ID of the room.
    room_id TEXT NOT NULL,
    -- The event ID of the m.room.member
    event_id TEXT NOT NULL,
    -- The domain part of the user ID the m.room.member event is for.
    server_name TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS joined_hosts_event_id_idx
    ON joined_hosts (event_id);

CREATE INDEX IF NOT EXISTS joined_hosts_room_id_idx
    ON joined_hosts (room_id)
`

const insertJoinedHostsSQL = "" +
	"INSERT INTO joined_hosts (room_id, event_id, server_name)" +
	" VALUES ($1, $2, $3)"

const deleteJoinedHostsSQL = "" +
	"DELETE FROM joined_hosts WHERE event_id = ANY($1)"

const selectJoinedHostsSQL = "" +
	"SELECT event_id, server_name FROM joined_hosts" +
	" WHERE room_id = $1"

type joinedHostsStatements struct {
	insertJoinedHostsStmt *sql.Stmt
	deleteJoinedHostsStmt *sql.Stmt
	selectJoinedHostsStmt *sql.Stmt
}

func (s *joinedHostsStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(joinedHostsSchema)
	if err != nil {
		return
	}
	if s.insertJoinedHostsStmt, err = db.Prepare(insertJoinedHostsSQL); err != nil {
		return
	}
	if s.deleteJoinedHostsStmt, err = db.Prepare(deleteJoinedHostsSQL); err != nil {
		return
	}
	if s.selectJoinedHostsStmt, err = db.Prepare(selectJoinedHostsSQL); err != nil {
		return
	}
	return
}

func (s *joinedHostsStatements) insertJoinedHosts(
	txn *sql.Tx, roomID, eventID string, serverName gomatrixserverlib.ServerName,
) error {
	_, err := txn.Stmt(s.insertJoinedHostsStmt).Exec(roomID, eventID, serverName)
	return err
}

func (s *joinedHostsStatements) deleteJoinedHosts(txn *sql.Tx, eventIDs []string) error {
	_, err := txn.Stmt(s.deleteJoinedHostsStmt).Exec(pq.StringArray(eventIDs))
	return err
}

func (s *joinedHostsStatements) selectJoinedHosts(txn *sql.Tx, roomID string,
) ([]types.JoinedHost, error) {
	rows, err := txn.Stmt(s.selectJoinedHostsStmt).Query(roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []types.JoinedHost
	for rows.Next() {
		var eventID, serverName string
		if err = rows.Scan(&eventID, &serverName); err != nil {
			return nil, err
		}
		result = append(result, types.JoinedHost{
			EventID:    eventID,
			ServerName: gomatrixserverlib.ServerName(serverName),
		})
	}
	return result, nil
}
