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

package deltas

import (
	"context"
	"database/sql"
	"fmt"
)

func UpPulishedAppservice(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `	ALTER TABLE roomserver_published RENAME TO roomserver_published_tmp;
CREATE TABLE IF NOT EXISTS roomserver_published (
    room_id TEXT NOT NULL,
    appservice_id TEXT NOT NULL DEFAULT '',
    network_id TEXT NOT NULL DEFAULT '',
    published BOOLEAN NOT NULL DEFAULT false,
    CONSTRAINT unique_published_idx PRIMARY KEY (room_id, appservice_id, network_id)
);
INSERT
    INTO roomserver_published (
      room_id, published
    ) SELECT
        room_id, published
    FROM roomserver_published_tmp
;
DROP TABLE roomserver_published_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownPublishedAppservice(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `	ALTER TABLE roomserver_published RENAME TO roomserver_published_tmp;
CREATE TABLE IF NOT EXISTS roomserver_published (
    room_id TEXT NOT NULL PRIMARY KEY,
    published BOOLEAN NOT NULL DEFAULT false
);
INSERT
    INTO roomserver_published (
      room_id, published
    ) SELECT
        room_id, published
    FROM roomserver_published_tmp
;
DROP TABLE roomserver_published_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}
