// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package sqlite3

import (
	"context"
	"database/sql"

	"github.com/element-hq/dendrite/federationapi/storage/shared"
	"github.com/element-hq/dendrite/federationapi/storage/sqlite3/deltas"
	"github.com/element-hq/dendrite/internal/caching"
	"github.com/element-hq/dendrite/internal/sqlutil"
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
func NewDatabase(ctx context.Context, conMan *sqlutil.Connections, dbProperties *config.DatabaseOptions, cache caching.FederationCache, isLocalServerName func(spec.ServerName) bool) (*Database, error) {
	var d Database
	var err error
	if d.db, d.writer, err = conMan.Connection(dbProperties); err != nil {
		return nil, err
	}
	blacklist, err := NewSQLiteBlacklistTable(d.db)
	if err != nil {
		return nil, err
	}
	joinedHosts, err := NewSQLiteJoinedHostsTable(d.db)
	if err != nil {
		return nil, err
	}
	queuePDUs, err := NewSQLiteQueuePDUsTable(d.db)
	if err != nil {
		return nil, err
	}
	queueEDUs, err := NewSQLiteQueueEDUsTable(d.db)
	if err != nil {
		return nil, err
	}
	queueJSON, err := NewSQLiteQueueJSONTable(d.db)
	if err != nil {
		return nil, err
	}
	assumedOffline, err := NewSQLiteAssumedOfflineTable(d.db)
	if err != nil {
		return nil, err
	}
	relayServers, err := NewSQLiteRelayServersTable(d.db)
	if err != nil {
		return nil, err
	}
	outboundPeeks, err := NewSQLiteOutboundPeeksTable(d.db)
	if err != nil {
		return nil, err
	}
	inboundPeeks, err := NewSQLiteInboundPeeksTable(d.db)
	if err != nil {
		return nil, err
	}
	notaryKeys, err := NewSQLiteNotaryServerKeysTable(d.db)
	if err != nil {
		return nil, err
	}
	notaryKeysMetadata, err := NewSQLiteNotaryServerKeysMetadataTable(d.db)
	if err != nil {
		return nil, err
	}
	serverSigningKeys, err := NewSQLiteServerSigningKeysTable(d.db)
	if err != nil {
		return nil, err
	}
	m := sqlutil.NewMigrator(d.db)
	m.AddMigrations(sqlutil.Migration{
		Version: "federationsender: drop federationsender_rooms",
		Up:      deltas.UpRemoveRoomsTable,
	})
	err = m.Up(ctx)
	if err != nil {
		return nil, err
	}
	if err = queueEDUs.Prepare(); err != nil {
		return nil, err
	}
	d.Database = shared.Database{
		DB:                       d.db,
		IsLocalServerName:        isLocalServerName,
		Cache:                    cache,
		Writer:                   d.writer,
		FederationJoinedHosts:    joinedHosts,
		FederationQueuePDUs:      queuePDUs,
		FederationQueueEDUs:      queueEDUs,
		FederationQueueJSON:      queueJSON,
		FederationBlacklist:      blacklist,
		FederationAssumedOffline: assumedOffline,
		FederationRelayServers:   relayServers,
		FederationOutboundPeeks:  outboundPeeks,
		FederationInboundPeeks:   inboundPeeks,
		NotaryServerKeysJSON:     notaryKeys,
		NotaryServerKeysMetadata: notaryKeysMetadata,
		ServerSigningKeys:        serverSigningKeys,
	}
	return &d, nil
}
