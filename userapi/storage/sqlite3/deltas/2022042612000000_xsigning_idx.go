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
		CREATE TABLE IF NOT EXISTS keyserver_cross_signing_sigs_tmp (
			origin_user_id TEXT NOT NULL,
			origin_key_id TEXT NOT NULL,
			target_user_id TEXT NOT NULL,
			target_key_id TEXT NOT NULL,
			signature TEXT NOT NULL,
			PRIMARY KEY (origin_user_id, origin_key_id, target_user_id, target_key_id)
		);

		INSERT INTO keyserver_cross_signing_sigs_tmp (origin_user_id, origin_key_id, target_user_id, target_key_id, signature)
			SELECT origin_user_id, origin_key_id, target_user_id, target_key_id, signature FROM keyserver_cross_signing_sigs;

		DROP TABLE keyserver_cross_signing_sigs;
		ALTER TABLE keyserver_cross_signing_sigs_tmp RENAME TO keyserver_cross_signing_sigs;

		CREATE INDEX IF NOT EXISTS keyserver_cross_signing_sigs_idx ON keyserver_cross_signing_sigs (origin_user_id, target_user_id, target_key_id);
	`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownFixCrossSigningSignatureIndexes(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS keyserver_cross_signing_sigs_tmp (
			origin_user_id TEXT NOT NULL,
			origin_key_id TEXT NOT NULL,
			target_user_id TEXT NOT NULL,
			target_key_id TEXT NOT NULL,
			signature TEXT NOT NULL,
			PRIMARY KEY (origin_user_id, target_user_id, target_key_id)
		);

		INSERT INTO keyserver_cross_signing_sigs_tmp (origin_user_id, origin_key_id, target_user_id, target_key_id, signature)
			SELECT origin_user_id, origin_key_id, target_user_id, target_key_id, signature FROM keyserver_cross_signing_sigs;

		DROP TABLE keyserver_cross_signing_sigs;
		ALTER TABLE keyserver_cross_signing_sigs_tmp RENAME TO keyserver_cross_signing_sigs;
		
		DELETE INDEX IF EXISTS keyserver_cross_signing_sigs_idx;
	`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
