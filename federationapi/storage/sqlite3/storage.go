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

package sqlite3

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/federationapi/storage/shared"
	"github.com/matrix-org/dendrite/federationapi/storage/sqlite3/deltas"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
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
