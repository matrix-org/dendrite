// Copyright 2023 The Matrix.org Foundation C.I.C.
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

func UpDropEventReferenceSHA(ctx context.Context, tx *sql.Tx) error {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM roomserver_events GROUP BY event_id HAVING count(event_id) > 1`).
		Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to query duplicate event ids")
	}
	if count > 0 {
		return fmt.Errorf("unable to drop column, as there are duplicate event ids")
	}
	_, err = tx.ExecContext(ctx, `ALTER TABLE roomserver_events DROP COLUMN reference_sha256;`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func UpDropEventReferenceSHAPrevEvents(ctx context.Context, tx *sql.Tx) error {
	// rename the table
	if _, err := tx.ExecContext(ctx, `ALTER TABLE roomserver_previous_events RENAME TO _roomserver_previous_events;`); err != nil {
		return fmt.Errorf("tx.ExecContext: %w", err)
	}

	// create new table
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS roomserver_previous_events (
    previous_event_id TEXT NOT NULL,
    event_nids TEXT NOT NULL,
    UNIQUE (previous_event_id)
  );`); err != nil {
		return fmt.Errorf("tx.ExecContext: %w", err)
	}

	// move data
	if _, err := tx.ExecContext(ctx, `
INSERT
    INTO roomserver_previous_events (
      previous_event_id, event_nids
    ) SELECT
        previous_event_id, event_nids
    FROM _roomserver_previous_events
;`); err != nil {
		return fmt.Errorf("tx.ExecContext: %w", err)
	}
	// drop old table
	_, err := tx.ExecContext(ctx, `DROP TABLE _roomserver_previous_events;`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}
