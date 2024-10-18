// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

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
