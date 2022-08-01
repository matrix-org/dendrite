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
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
)

func UpAddHistoryVisibilityColumnOutputRoomEvents(ctx context.Context, tx *sql.Tx) error {
	// SQLite doesn't have "if exists", so check if the column exists. If the query doesn't return an error, it already exists.
	// Required for unit tests, as otherwise a duplicate column error will show up.
	_, err := tx.QueryContext(ctx, "SELECT history_visibility FROM syncapi_output_room_events LIMIT 1")
	if err == nil {
		return nil
	}
	_, err = tx.ExecContext(ctx, `
		ALTER TABLE syncapi_output_room_events ADD COLUMN history_visibility SMALLINT NOT NULL DEFAULT 2;
		UPDATE syncapi_output_room_events SET history_visibility = 4 WHERE type IN ('m.room.message', 'm.room.encrypted');
	`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

// UpSetHistoryVisibility sets the history visibility for already stored events.
// Requires current_room_state and output_room_events to be created.
func UpSetHistoryVisibility(ctx context.Context, tx *sql.Tx) error {
	// get the current room history visibilities
	historyVisibilities, err := currentHistoryVisibilities(ctx, tx)
	if err != nil {
		return err
	}

	// update the history visibility
	for roomID, hisVis := range historyVisibilities {
		_, err = tx.ExecContext(ctx, `UPDATE syncapi_output_room_events SET history_visibility = $1 
                        WHERE type IN ('m.room.message', 'm.room.encrypted') AND room_id = $2 AND history_visibility <> $1`, hisVis, roomID)
		if err != nil {
			return fmt.Errorf("failed to update history visibility: %w", err)
		}
	}

	return nil
}

func UpAddHistoryVisibilityColumnCurrentRoomState(ctx context.Context, tx *sql.Tx) error {
	// SQLite doesn't have "if exists", so check if the column exists. If the query doesn't return an error, it already exists.
	// Required for unit tests, as otherwise a duplicate column error will show up.
	_, err := tx.QueryContext(ctx, "SELECT history_visibility FROM syncapi_current_room_state LIMIT 1")
	if err == nil {
		return nil
	}
	_, err = tx.ExecContext(ctx, `
		ALTER TABLE syncapi_current_room_state ADD COLUMN history_visibility SMALLINT NOT NULL DEFAULT 2;
		UPDATE syncapi_current_room_state SET history_visibility = 4 WHERE type IN ('m.room.message', 'm.room.encrypted');
	`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}

	return nil
}

// currentHistoryVisibilities returns a map from roomID to current history visibility.
// If the history visibility was changed after room creation, defaults to joined.
func currentHistoryVisibilities(ctx context.Context, tx *sql.Tx) (map[string]gomatrixserverlib.HistoryVisibility, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT room_id, headered_event_json FROM syncapi_current_room_state
		WHERE type = 'm.room.history_visibility' AND state_key = '';
`)
	if err != nil {
		return nil, fmt.Errorf("failed to query current room state: %w", err)
	}
	defer rows.Close() // nolint: errcheck
	var eventBytes []byte
	var roomID string
	var event gomatrixserverlib.HeaderedEvent
	var hisVis gomatrixserverlib.HistoryVisibility
	historyVisibilities := make(map[string]gomatrixserverlib.HistoryVisibility)
	for rows.Next() {
		if err = rows.Scan(&roomID, &eventBytes); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		if err = json.Unmarshal(eventBytes, &event); err != nil {
			return nil, fmt.Errorf("failed to unmarshal event: %w", err)
		}
		historyVisibilities[roomID] = gomatrixserverlib.HistoryVisibilityJoined
		if hisVis, err = event.HistoryVisibility(); err == nil && event.Depth() < 10 {
			historyVisibilities[roomID] = hisVis
		}
	}
	return historyVisibilities, nil
}

func DownAddHistoryVisibilityColumn(ctx context.Context, tx *sql.Tx) error {
	// SQLite doesn't have "if exists", so check if the column exists.
	_, err := tx.QueryContext(ctx, "SELECT history_visibility FROM syncapi_output_room_events LIMIT 1")
	if err != nil {
		// The column probably doesn't exist
		return nil
	}
	_, err = tx.ExecContext(ctx, `
		ALTER TABLE syncapi_output_room_events DROP COLUMN history_visibility;
	`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	_, err = tx.QueryContext(ctx, "SELECT history_visibility FROM syncapi_current_room_state LIMIT 1")
	if err != nil {
		// The column probably doesn't exist
		return nil
	}
	_, err = tx.ExecContext(ctx, `
		ALTER TABLE syncapi_current_room_state DROP COLUMN history_visibility;
	`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
