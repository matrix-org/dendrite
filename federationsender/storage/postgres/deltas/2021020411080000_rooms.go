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
	"github.com/pressly/goose"
)

func LoadFromGoose() {
	goose.AddMigration(UpRemoveRoomsTable, DownRemoveRoomsTable)
}

func LoadRemoveRoomsTable(m *sqlutil.Migrations) {
	m.AddMigration(UpRemoveRoomsTable, DownRemoveRoomsTable)
}

func UpRemoveRoomsTable(tx *sql.Tx) error {
	_, err := tx.Exec(`
		DROP TABLE IF EXISTS federationsender_rooms;
	`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownRemoveRoomsTable(tx *sql.Tx) error {
	// We can't reverse this.
	return nil
}
