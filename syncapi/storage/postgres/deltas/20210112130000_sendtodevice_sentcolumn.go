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
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
)

func LoadRemoveSendToDeviceSentColumn(m *sqlutil.Migrations) {
	m.AddMigration(UpRemoveSendToDeviceSentColumn, DownRemoveSendToDeviceSentColumn)
}

func UpRemoveSendToDeviceSentColumn(tx *sql.Tx) error {
	_, err := tx.Exec(`
		ALTER TABLE syncapi_send_to_device
		  DROP COLUMN IF EXISTS sent_by_token;
	`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownRemoveSendToDeviceSentColumn(tx *sql.Tx) error {
	_, err := tx.Exec(`
		ALTER TABLE syncapi_send_to_device
		  ADD COLUMN IF NOT EXISTS sent_by_token TEXT;
	`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
