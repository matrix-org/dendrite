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
	"time"

	"github.com/matrix-org/gomatrixserverlib"
)

func UpAddexpiresat(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, "ALTER TABLE federationsender_queue_edus RENAME TO federationsender_queue_edus_old;")
	if err != nil {
		return fmt.Errorf("failed to rename table: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS federationsender_queue_edus (
	edu_type TEXT NOT NULL,
	server_name TEXT NOT NULL,
	json_nid BIGINT NOT NULL,
	expires_at BIGINT NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS federationsender_queue_edus_json_nid_idx
    ON federationsender_queue_edus (json_nid, server_name);
`)
	if err != nil {
		return fmt.Errorf("failed to create new table: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT
    INTO federationsender_queue_edus (
        edu_type, server_name, json_nid, expires_at
    )  SELECT edu_type, server_name, json_nid, 0 FROM federationsender_queue_edus_old;
`)
	if err != nil {
		return fmt.Errorf("failed to update queue_edus: %w", err)
	}
	_, err = tx.ExecContext(ctx, "UPDATE federationsender_queue_edus SET expires_at = $1 WHERE edu_type != 'm.direct_to_device'", gomatrixserverlib.AsTimestamp(time.Now().Add(time.Hour*24)))
	if err != nil {
		return fmt.Errorf("failed to update queue_edus: %w", err)
	}
	return nil
}

func DownAddexpiresat(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, "ALTER TABLE federationsender_queue_edus DROP COLUMN expires_at;")
	if err != nil {
		return fmt.Errorf("failed to rename table: %w", err)
	}
	return nil
}
