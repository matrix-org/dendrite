// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only
// Please see LICENSE in the repository root for full details.

package storage

import (
	"context"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/storage/sqlite3"
)

// NewPublicRoomsServerDatabase opens a database connection.
func NewSyncServerDatasource(ctx context.Context, conMan sqlutil.Connections, dbProperties *config.DatabaseOptions) (Database, error) {
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		return sqlite3.NewDatabase(ctx, conMan, dbProperties)
	case dbProperties.ConnectionString.IsPostgres():
		return nil, fmt.Errorf("can't use Postgres implementation")
	default:
		return nil, fmt.Errorf("unexpected database type")
	}
}
