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

func LoadAddHistoryVisibilityColumn(m *sqlutil.Migrations) {
	m.AddMigration(UpAddHistoryVisibilityColumn, DownAddHistoryVisibilityColumn)
}

func UpAddHistoryVisibilityColumn(tx *sql.Tx) error {
	_, err := tx.Exec(`
		ALTER TABLE syncapi_output_room_events ADD COLUMN IF NOT EXISTS history_visibility SMALLINT NOT NULL DEFAULT 2;
		UPDATE syncapi_output_room_events SET history_visibility = 4 WHERE type IN ('m.room.message', 'm.room.encrypted');
	`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownAddHistoryVisibilityColumn(tx *sql.Tx) error {
	_, err := tx.Exec(`
		ALTER TABLE syncapi_output_room_events DROP COLUMN IF EXISTS history_visibility;
	`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
