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
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/pressly/goose"
)

func LoadFromGoose() {
	goose.AddMigration(UpFixSequences, DownFixSequences)
	goose.AddMigration(UpRemoveSendToDeviceSentColumn, DownRemoveSendToDeviceSentColumn)
}

func LoadFixSequences(m *sqlutil.Migrations) {
	m.AddMigration(UpFixSequences, DownFixSequences)
}

func UpFixSequences(tx *sql.Tx) error {
	_, err := tx.Exec(`
		-- We need to delete all of the existing receipts because the indexes
		-- will be wrong, and we'll get primary key violations if we try to
		-- reuse existing stream IDs from a different sequence.
		DELETE FROM syncapi_receipts;

		-- Use the new syncapi_receipts_id sequence.
		CREATE SEQUENCE IF NOT EXISTS syncapi_receipt_id;
		ALTER SEQUENCE IF EXISTS syncapi_receipt_id RESTART WITH 1;
		ALTER TABLE syncapi_receipts ALTER COLUMN id SET DEFAULT nextval('syncapi_receipt_id');
	`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownFixSequences(tx *sql.Tx) error {
	_, err := tx.Exec(`
		-- We need to delete all of the existing receipts because the indexes
		-- will be wrong, and we'll get primary key violations if we try to
		-- reuse existing stream IDs from a different sequence.
		DELETE FROM syncapi_receipts;

		-- Revert back to using the syncapi_stream_id sequence.
		DROP SEQUENCE IF EXISTS syncapi_receipt_id;
		ALTER TABLE syncapi_receipts ALTER COLUMN id SET DEFAULT nextval('syncapi_stream_id');
	`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
