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
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
)

func LoadAddExpiresAt(m *sqlutil.Migrations) {
	m.AddMigration(upAddexpiresat, downAddexpiresat)
}

func upAddexpiresat(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE federationsender_queue_edus RENAME TO federationsender_queue_edus_old;")
	if err != nil {
		return fmt.Errorf("failed to rename table: %w", err)
	}

	_, err = tx.Exec(`
CREATE TABLE IF NOT EXISTS federationsender_queue_edus (
	edu_type TEXT NOT NULL,
	server_name TEXT NOT NULL,
	json_nid BIGINT NOT NULL,
	expires_at BIGINT
);

CREATE UNIQUE INDEX IF NOT EXISTS federationsender_queue_edus_json_nid_idx
    ON federationsender_queue_edus (json_nid, server_name);
`)
	if err != nil {
		return fmt.Errorf("failed to create new table: %w", err)
	}
	_, err = tx.Exec(`
INSERT
    INTO federationsender_queue_edus (
        edu_type, server_name, json_nid, expires_at
    )  SELECT edu_type, server_name, json_nid, null FROM federationsender_queue_edus_old;
`)
	if err != nil {
		return fmt.Errorf("failed to move data to new table: %w", err)
	}

	_, err = tx.Exec("DROP TABLE federationsender_queue_edus_old;")
	if err != nil {
		return fmt.Errorf("failed to create new table: %w", err)
	}

	return nil
}

func downAddexpiresat(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE federationsender_queue_edus RENAME TO federationsender_queue_edus_old;")
	if err != nil {
		return fmt.Errorf("failed to rename table: %w", err)
	}

	_, err = tx.Exec(`
CREATE TABLE IF NOT EXISTS federationsender_queue_edus (
	edu_type TEXT NOT NULL,
	server_name TEXT NOT NULL,
	json_nid BIGINT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS federationsender_queue_edus_json_nid_idx
    ON federationsender_queue_edus (json_nid, server_name);
`)
	if err != nil {
		return fmt.Errorf("failed to create new table: %w", err)
	}
	_, err = tx.Exec(`
INSERT
    INTO federationsender_queue_edus (
        edu_type, server_name, json_nid
    )  SELECT edu_type, server_name, json_nid FROM federationsender_queue_edus_old;
`)
	if err != nil {
		return fmt.Errorf("failed to move data to new table: %w", err)
	}

	_, err = tx.Exec("DROP TABLE federationsender_queue_edus_old;")
	if err != nil {
		return fmt.Errorf("failed to create new table: %w", err)
	}

	return nil
}
