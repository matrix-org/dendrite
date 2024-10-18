// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package deltas

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

func UpAddHistoryVisibilityColumnOutputRoomEvents(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		ALTER TABLE syncapi_output_room_events ADD COLUMN IF NOT EXISTS history_visibility SMALLINT NOT NULL DEFAULT 2;
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
	_, err := tx.ExecContext(ctx, `
		ALTER TABLE syncapi_current_room_state ADD COLUMN IF NOT EXISTS history_visibility SMALLINT NOT NULL DEFAULT 2;
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
	var event types.HeaderedEvent
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
	_, err := tx.ExecContext(ctx, `
		ALTER TABLE syncapi_output_room_events DROP COLUMN IF EXISTS history_visibility;
		ALTER TABLE syncapi_current_room_state DROP COLUMN IF EXISTS history_visibility;
	`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
