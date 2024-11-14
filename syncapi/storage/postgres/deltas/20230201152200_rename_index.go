// Copyright 2024 New Vector Ltd.
// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package deltas

import (
	"context"
	"database/sql"
	"fmt"
)

func UpRenameOutputRoomEventsIndex(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `ALTER TABLE syncapi_output_room_events RENAME CONSTRAINT syncapi_event_id_idx TO syncapi_output_room_event_id_idx;`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}
