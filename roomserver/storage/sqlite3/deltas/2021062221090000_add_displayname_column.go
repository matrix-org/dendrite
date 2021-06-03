// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package deltas

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/types"
)

func LoadAddDisplaynameColumn(m *sqlutil.Migrations) {
	m.AddMigration(UpAddDisplaynameColumn, DownAddDisplaynameColumn)
}

func UpAddDisplaynameColumn(tx *sql.Tx) error {
	_, err := tx.Exec(`	ALTER TABLE roomserver_membership RENAME TO roomserver_membership_tmp;
CREATE TABLE IF NOT EXISTS roomserver_membership (
	room_nid INTEGER NOT NULL,
	target_nid INTEGER NOT NULL,
	sender_nid INTEGER NOT NULL DEFAULT 0,
	membership_nid INTEGER NOT NULL DEFAULT 1,
	event_nid INTEGER NOT NULL DEFAULT 0,
	target_local BOOLEAN NOT NULL DEFAULT false,
	forgotten BOOLEAN NOT NULL DEFAULT false,
	displayname TEXT NOT NULL DEFAULT '',
	UNIQUE (room_nid, target_nid)
);
INSERT
    INTO roomserver_membership (
      room_nid, target_nid, sender_nid, membership_nid, event_nid, target_local, forgotten
    ) SELECT
        room_nid, target_nid, sender_nid, membership_nid, event_nid, target_local, forgotten
    FROM roomserver_membership_tmp
;
DROP TABLE roomserver_membership_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}

	rows, err := tx.Query(`SELECT room_nid, target_nid, event_json FROM roomserver_membership 
INNER JOIN roomserver_event_json ON 
	roomserver_membership.event_nid = roomserver_event_json.event_nid
WHERE displayname = '';
`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}

	for rows.Next() {
		var roomNID types.RoomNID
		var targetUserNID types.EventStateKeyNID
		var eventJSONRaw []byte

		if err = rows.Scan(&roomNID, &targetUserNID, &eventJSONRaw); err != nil {
			return fmt.Errorf("error scanning row: %v", err)
		}

		eventJSON := make(map[string]interface{})
		err = json.Unmarshal(eventJSONRaw, &eventJSON)
		if err != nil {
			return fmt.Errorf("could not unmarshal event JSON: %v", err)
		}

		eventDisplayname := ""
		if content, ok := eventJSON["content"]; ok {
			if displayname, ok := content.(map[string]interface{})["displayname"]; ok {
				if displaynameString, ok := displayname.(string); ok {
					eventDisplayname = displaynameString
				}
			}
		}
		if eventDisplayname != "" {
			_, err = tx.Exec(`UPDATE roomserver_membership 
SET displayname = $1 
WHERE room_nid = $2 AND target_nid = $3;`,
				eventDisplayname, roomNID, targetUserNID)
			if err != nil {
				return fmt.Errorf("could not update displayname field: %v", err)
			}
		}

	}

	return nil
}

func DownAddDisplaynameColumn(tx *sql.Tx) error {
	_, err := tx.Exec(`	ALTER TABLE roomserver_membership RENAME TO roomserver_membership_tmp;
CREATE TABLE IF NOT EXISTS roomserver_membership (
	room_nid INTEGER NOT NULL,
	target_nid INTEGER NOT NULL,
	sender_nid INTEGER NOT NULL DEFAULT 0,
	membership_nid INTEGER NOT NULL DEFAULT 1,
	event_nid INTEGER NOT NULL DEFAULT 0,
	target_local BOOLEAN NOT NULL DEFAULT false,
	forgotten BOOLEAN NOT NULL DEFAULT false,
	UNIQUE (room_nid, target_nid)
);
INSERT
    INTO roomserver_membership (
      room_nid, target_nid, sender_nid, membership_nid, event_nid, target_local, forgotten
    ) SELECT
        room_nid, target_nid, sender_nid, membership_nid, event_nid, target_local, forgotten
    FROM roomserver_membership_tmp
;
DROP TABLE roomserver_membership_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
