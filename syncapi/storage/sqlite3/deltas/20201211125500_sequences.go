// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package deltas

import (
	"context"
	"database/sql"
	"fmt"
)

func UpFixSequences(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		-- We need to delete all of the existing receipts because the indexes
		-- will be wrong, and we'll get primary key violations if we try to
		-- reuse existing stream IDs from a different sequence.
		DELETE FROM syncapi_receipts;
		UPDATE syncapi_stream_id SET stream_id=1 WHERE stream_name="receipt";
	`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownFixSequences(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		-- We need to delete all of the existing receipts because the indexes
		-- will be wrong, and we'll get primary key violations if we try to
		-- reuse existing stream IDs from a different sequence.
		DELETE FROM syncapi_receipts;
	`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
