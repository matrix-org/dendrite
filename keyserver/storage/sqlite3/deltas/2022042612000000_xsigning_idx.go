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
