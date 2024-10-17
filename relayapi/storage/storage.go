// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only
// Please see LICENSE in the repository root for full details.

//go:build !wasm
// +build !wasm

package storage

import (
	"fmt"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/relayapi/storage/postgres"
	"github.com/matrix-org/dendrite/relayapi/storage/sqlite3"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

// NewDatabase opens a new database
func NewDatabase(
	conMan *sqlutil.Connections,
	dbProperties *config.DatabaseOptions,
	cache caching.FederationCache,
	isLocalServerName func(spec.ServerName) bool,
) (Database, error) {
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		return sqlite3.NewDatabase(conMan, dbProperties, cache, isLocalServerName)
	case dbProperties.ConnectionString.IsPostgres():
		return postgres.NewDatabase(conMan, dbProperties, cache, isLocalServerName)
	default:
		return nil, fmt.Errorf("unexpected database type")
	}
}
