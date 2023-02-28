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

func UpRefactorKeyChanges(ctx context.Context, tx *sql.Tx) error {
	// start counting from the last max offset, else 0.
	var maxOffset int64
	var userID string
	_ = tx.QueryRowContext(ctx, `SELECT user_id, MAX(log_offset) FROM keyserver_key_changes GROUP BY user_id`).Scan(&userID, &maxOffset)

	_, err := tx.ExecContext(ctx, `
		-- make the new table
		DROP TABLE IF EXISTS keyserver_key_changes;
		CREATE TABLE IF NOT EXISTS keyserver_key_changes (
			change_id INTEGER PRIMARY KEY AUTOINCREMENT,
			-- The key owner
			user_id TEXT NOT NULL,
			UNIQUE (user_id)
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	// to start counting from maxOffset, insert a row with that value
	if userID != "" {
		_, err = tx.ExecContext(ctx, `INSERT INTO keyserver_key_changes(change_id, user_id) VALUES($1, $2)`, maxOffset, userID)
		return err
	}
	return nil
}

func DownRefactorKeyChanges(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
	-- Drop all data and revert back, we can't keep the data as Kafka offsets determine the numbers
	DROP TABLE IF EXISTS keyserver_key_changes;
	CREATE TABLE IF NOT EXISTS keyserver_key_changes (
		partition BIGINT NOT NULL,
		offset BIGINT NOT NULL,
		-- The key owner
		user_id TEXT NOT NULL,
		UNIQUE (partition, offset)
	);
	`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
