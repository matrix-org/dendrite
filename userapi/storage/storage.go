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
	"time"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/userapi/storage/postgres"
	"github.com/element-hq/dendrite/userapi/storage/sqlite3"
)

// NewUserDatabase opens a new Postgres or Sqlite database (based on dataSourceName scheme)
// and sets postgres connection parameters
func NewUserDatabase(
	ctx context.Context,
	conMan *sqlutil.Connections,
	dbProperties *config.DatabaseOptions,
	serverName spec.ServerName,
	bcryptCost int,
	openIDTokenLifetimeMS int64,
	loginTokenLifetime time.Duration,
	serverNoticesLocalpart string,
) (UserDatabase, error) {
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		return sqlite3.NewUserDatabase(ctx, conMan, dbProperties, serverName, bcryptCost, openIDTokenLifetimeMS, loginTokenLifetime, serverNoticesLocalpart)
	case dbProperties.ConnectionString.IsPostgres():
		return postgres.NewDatabase(ctx, conMan, dbProperties, serverName, bcryptCost, openIDTokenLifetimeMS, loginTokenLifetime, serverNoticesLocalpart)
	default:
		return nil, fmt.Errorf("unexpected database type")
	}
}

// NewKeyDatabase opens a new Postgres or Sqlite database (base on dataSourceName) scheme)
// and sets postgres connection parameters.
func NewKeyDatabase(conMan *sqlutil.Connections, dbProperties *config.DatabaseOptions) (KeyDatabase, error) {
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		return sqlite3.NewKeyDatabase(conMan, dbProperties)
	case dbProperties.ConnectionString.IsPostgres():
		return postgres.NewKeyDatabase(conMan, dbProperties)
	default:
		return nil, fmt.Errorf("unexpected database type")
	}
}
