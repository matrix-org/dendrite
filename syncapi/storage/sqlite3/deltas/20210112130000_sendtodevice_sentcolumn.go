// Copyright 2021 The Matrix.org Foundation C.I.C.
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
