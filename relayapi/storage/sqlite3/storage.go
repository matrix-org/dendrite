// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package sqlite3

import (
	"database/sql"

	"github.com/element-hq/dendrite/internal/caching"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/relayapi/storage/shared"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

// Database stores information needed by the federation sender
type Database struct {
	shared.Database
	db     *sql.DB
	writer sqlutil.Writer
}

// NewDatabase opens a new database
func NewDatabase(
	conMan *sqlutil.Connections,
	dbProperties *config.DatabaseOptions,
	cache caching.FederationCache,
	isLocalServerName func(spec.ServerName) bool,
) (*Database, error) {
	var d Database
	var err error
	if d.db, d.writer, err = conMan.Connection(dbProperties); err != nil {
		return nil, err
	}
	queue, err := NewSQLiteRelayQueueTable(d.db)
	if err != nil {
		return nil, err
	}
	queueJSON, err := NewSQLiteRelayQueueJSONTable(d.db)
	if err != nil {
		return nil, err
	}
	d.Database = shared.Database{
		DB:                d.db,
		IsLocalServerName: isLocalServerName,
		Cache:             cache,
		Writer:            d.writer,
		RelayQueue:        queue,
		RelayQueueJSON:    queueJSON,
	}
	return &d, nil
}
