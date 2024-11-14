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

func UpPulishedAppservicePrimaryKey(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `ALTER TABLE roomserver_published RENAME CONSTRAINT roomserver_published_pkey TO roomserver_published_pkeyold;
CREATE UNIQUE INDEX roomserver_published_pkey ON roomserver_published (room_id, appservice_id, network_id);
ALTER TABLE roomserver_published DROP CONSTRAINT roomserver_published_pkeyold;
ALTER TABLE roomserver_published ADD PRIMARY KEY USING INDEX roomserver_published_pkey;`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}
