// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package deltas

import (
	"context"
	"database/sql"
	"fmt"
)

func UpFixCrossSigningSignatureIndexes(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		ALTER TABLE keyserver_cross_signing_sigs DROP CONSTRAINT keyserver_cross_signing_sigs_pkey;
		ALTER TABLE keyserver_cross_signing_sigs ADD PRIMARY KEY (origin_user_id, origin_key_id, target_user_id, target_key_id);

		CREATE INDEX IF NOT EXISTS keyserver_cross_signing_sigs_idx ON keyserver_cross_signing_sigs (origin_user_id, target_user_id, target_key_id);
	`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownFixCrossSigningSignatureIndexes(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		ALTER TABLE keyserver_cross_signing_sigs DROP CONSTRAINT keyserver_cross_signing_sigs_pkey;
		ALTER TABLE keyserver_cross_signing_sigs ADD PRIMARY KEY (origin_user_id, target_user_id, target_key_id);

		DROP INDEX IF EXISTS keyserver_cross_signing_sigs_idx;
	`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
