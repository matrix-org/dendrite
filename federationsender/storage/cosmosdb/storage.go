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

package cosmosdb

import (
	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	_ "github.com/mattn/go-sqlite3"

	"github.com/matrix-org/dendrite/federationsender/storage/shared"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/setup/config"
)

// Database stores information needed by the federation sender
type Database struct {
	shared.Database
	cosmosdbutil.PartitionOffsetStatements
	database     cosmosdbutil.Database
	writer       cosmosdbutil.Writer
	connection   cosmosdbapi.CosmosConnection
	databaseName string
	cosmosConfig cosmosdbapi.CosmosConfig
}

// NewDatabase opens a new database
func NewDatabase(dbProperties *config.DatabaseOptions, cache caching.FederationSenderCache) (*Database, error) {
	conn := cosmosdbutil.GetCosmosConnection(&dbProperties.ConnectionString)
	configCosmos := cosmosdbutil.GetCosmosConfig(&dbProperties.ConnectionString)
	var d Database
	d.connection = conn
	d.cosmosConfig = configCosmos
	d.databaseName = "federationsender"
	d.database = cosmosdbutil.Database{
		Connection:   conn,
		CosmosConfig: configCosmos,
		DatabaseName: d.databaseName,
	}

	var err error

	d.writer = cosmosdbutil.NewExclusiveWriterFake()
	joinedHosts, err := NewCosmosDBJoinedHostsTable(&d)
	if err != nil {
		return nil, err
	}
	queuePDUs, err := NewCosmosDBQueuePDUsTable(&d)
	if err != nil {
		return nil, err
	}
	queueEDUs, err := NewCosmosDBQueueEDUsTable(&d)
	if err != nil {
		return nil, err
	}
	queueJSON, err := NewCosmosDBQueueJSONTable(&d)
	if err != nil {
		return nil, err
	}
	blacklist, err := NewCosmosDBBlacklistTable(&d)
	if err != nil {
		return nil, err
	}
	outboundPeeks, err := NewCosmosDBOutboundPeeksTable(&d)
	if err != nil {
		return nil, err
	}
	inboundPeeks, err := NewCosmosDBInboundPeeksTable(&d)
	if err != nil {
		return nil, err
	}
	d.Database = shared.Database{
		DB:                            nil,
		Cache:                         cache,
		Writer:                        d.writer,
		FederationSenderJoinedHosts:   joinedHosts,
		FederationSenderQueuePDUs:     queuePDUs,
		FederationSenderQueueEDUs:     queueEDUs,
		FederationSenderQueueJSON:     queueJSON,
		FederationSenderBlacklist:     blacklist,
		FederationSenderOutboundPeeks: outboundPeeks,
		FederationSenderInboundPeeks:  inboundPeeks,
	}
	if err = d.PartitionOffsetStatements.Prepare(&d.database, d.writer, "federationsender"); err != nil {
		return nil, err
	}
	return &d, nil
}
