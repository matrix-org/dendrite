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
	"github.com/pressly/goose"
)

func LoadFromGoose() {
	goose.AddMigration(UpRefactorKeyChanges, DownRefactorKeyChanges)
}

func LoadRefactorKeyChanges(m *sqlutil.Migrations) {
	m.AddMigration(UpRefactorKeyChanges, DownRefactorKeyChanges)
}

func UpRefactorKeyChanges(tx *sql.Tx) error {
	// start counting from the last max offset, else 0. We need to do a count(*) first to see if there
	// even are entries in this table to know if we can query for log_offset. Without the count then
	// the query to SELECT the max log offset fails on new Dendrite instances as log_offset doesn't
	// exist on that table. Even though we discard the error, the txn is tainted and gets aborted :/
	var count int
	_ = tx.QueryRow(`SELECT count(*) FROM keyserver_key_changes`).Scan(&count)
	if count > 0 {
		var maxOffset int64
		_ = tx.QueryRow(`SELECT coalesce(MAX(log_offset), 0) AS offset FROM keyserver_key_changes`).Scan(&maxOffset)
		if _, err := tx.Exec(fmt.Sprintf(`CREATE SEQUENCE IF NOT EXISTS keyserver_key_changes_seq START %d`, maxOffset)); err != nil {
			return fmt.Errorf("failed to CREATE SEQUENCE for key changes, starting at %d: %s", maxOffset, err)
		}
	}

	_, err := tx.Exec(`
		-- make the new table
		DROP TABLE IF EXISTS keyserver_key_changes;
		CREATE TABLE IF NOT EXISTS keyserver_key_changes (
			change_id BIGINT PRIMARY KEY DEFAULT nextval('keyserver_key_changes_seq'),
			user_id TEXT NOT NULL,
			CONSTRAINT keyserver_key_changes_unique_per_user UNIQUE (user_id)
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownRefactorKeyChanges(tx *sql.Tx) error {
	_, err := tx.Exec(`
	-- Drop all data and revert back, we can't keep the data as Kafka offsets determine the numbers
	DROP SEQUENCE IF EXISTS keyserver_key_changes_seq;
	DROP TABLE IF EXISTS keyserver_key_changes;
	CREATE TABLE IF NOT EXISTS keyserver_key_changes (
		partition BIGINT NOT NULL,
		log_offset BIGINT NOT NULL,
		user_id TEXT NOT NULL,
		CONSTRAINT keyserver_key_changes_unique UNIQUE (partition, log_offset)
	);
	`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
