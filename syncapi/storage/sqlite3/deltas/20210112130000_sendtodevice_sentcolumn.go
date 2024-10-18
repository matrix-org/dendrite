// Copyright 2024 New Vector Ltd.
// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package deltas

import (
	"context"
	"database/sql"
	"fmt"
)

func UpRemoveSendToDeviceSentColumn(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		CREATE TEMPORARY TABLE syncapi_send_to_device_backup(id, user_id, device_id, content);
		INSERT INTO syncapi_send_to_device_backup SELECT id, user_id, device_id, content FROM syncapi_send_to_device;
		DROP TABLE syncapi_send_to_device;
		CREATE TABLE syncapi_send_to_device(
			id INTEGER PRIMARY KEY AUTOINCREMENT, 
			user_id TEXT NOT NULL,
			device_id TEXT NOT NULL,
			content TEXT NOT NULL
		);
		INSERT INTO syncapi_send_to_device SELECT id, user_id, device_id, content FROM syncapi_send_to_device_backup;
		DROP TABLE syncapi_send_to_device_backup;
	`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownRemoveSendToDeviceSentColumn(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		CREATE TEMPORARY TABLE syncapi_send_to_device_backup(id, user_id, device_id, content);
		INSERT INTO syncapi_send_to_device_backup SELECT id, user_id, device_id, content FROM syncapi_send_to_device;
		DROP TABLE syncapi_send_to_device;
		CREATE TABLE syncapi_send_to_device(
			id INTEGER PRIMARY KEY AUTOINCREMENT, 
			user_id TEXT NOT NULL, 
			device_id TEXT NOT NULL, 
			content TEXT NOT NULL,
			sent_by_token TEXT
		);
		INSERT INTO syncapi_send_to_device SELECT id, user_id, device_id, content FROM syncapi_send_to_device_backup;
		DROP TABLE syncapi_send_to_device_backup;
	`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}
