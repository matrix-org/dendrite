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
	"context"
	"database/sql"
	"fmt"
)

func UpAddForgottenColumn(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `	ALTER TABLE roomserver_membership RENAME TO roomserver_membership_tmp;
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
      room_nid, target_nid, sender_nid, membership_nid, event_nid, target_local
    ) SELECT
        room_nid, target_nid, sender_nid, membership_nid, event_nid, target_local
    FROM roomserver_membership_tmp
;
DROP TABLE roomserver_membership_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownAddForgottenColumn(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `	ALTER TABLE roomserver_membership RENAME TO roomserver_membership_tmp;
CREATE TABLE IF NOT EXISTS roomserver_membership (
		room_nid INTEGER NOT NULL,
		target_nid INTEGER NOT NULL,
		sender_nid INTEGER NOT NULL DEFAULT 0,
		membership_nid INTEGER NOT NULL DEFAULT 1,
		event_nid INTEGER NOT NULL DEFAULT 0,
		target_local BOOLEAN NOT NULL DEFAULT false,
		UNIQUE (room_nid, target_nid)
	);
INSERT
    INTO roomserver_membership (
      room_nid, target_nid, sender_nid, membership_nid, event_nid, target_local
    ) SELECT
        room_nid, target_nid, sender_nid, membership_nid, event_nid, target_local
    FROM roomserver_membership_tmp
;
DROP TABLE roomserver_membership_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
