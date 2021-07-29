// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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
	"database/sql"

	"github.com/matrix-org/dendrite/federationsender/storage/shared"
	"github.com/matrix-org/dendrite/federationsender/storage/sqlite3/deltas"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
)

// Database stores information needed by the federation sender
type Database struct {
	shared.Database
	sqlutil.PartitionOffsetStatements
	db     *sql.DB
	writer sqlutil.Writer
}

// NewDatabase opens a new database
func NewDatabase(dbProperties *config.DatabaseOptions, cache caching.FederationSenderCache) (*Database, error) {
	var d Database
	var err error
	if d.db, err = sqlutil.Open(dbProperties); err != nil {
		return nil, err
	}
	d.writer = sqlutil.NewExclusiveWriter()
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
	blacklist, err := NewSQLiteBlacklistTable(d.db)
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
	m := sqlutil.NewMigrations()
	deltas.LoadRemoveRoomsTable(m)
	if err = m.RunDeltas(d.db, dbProperties); err != nil {
		return nil, err
	}
	d.Database = shared.Database{
		DB:                            d.db,
		Cache:                         cache,
		Writer:                        d.writer,
		FederationSenderJoinedHosts:   joinedHosts,
		FederationSenderQueuePDUs:     queuePDUs,
		FederationSenderQueueEDUs:     queueEDUs,
		FederationSenderQueueJSON:     queueJSON,
		FederationSenderBlacklist:     blacklist,
		FederationSenderOutboundPeeks: outboundPeeks,
		FederationSenderInboundPeeks:  inboundPeeks,
		NotaryServerKeysJSON:          notaryKeys,
		NotaryServerKeysMetadata:      notaryKeysMetadata,
	}
	if err = d.PartitionOffsetStatements.Prepare(d.db, d.writer, "federationsender"); err != nil {
		return nil, err
	}
	return &d, nil
}
