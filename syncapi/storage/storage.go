// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

//go:build !wasm
// +build !wasm

package storage

import (
	"context"
	"fmt"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/syncapi/storage/postgres"
	"github.com/element-hq/dendrite/syncapi/storage/sqlite3"
)

// NewSyncServerDatasource opens a database connection.
func NewSyncServerDatasource(ctx context.Context, conMan *sqlutil.Connections, dbProperties *config.DatabaseOptions) (Database, error) {
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		return sqlite3.NewDatabase(ctx, conMan, dbProperties)
	case dbProperties.ConnectionString.IsPostgres():
		return postgres.NewDatabase(ctx, conMan, dbProperties)
	default:
		return nil, fmt.Errorf("unexpected database type")
	}
}
