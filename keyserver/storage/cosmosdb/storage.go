// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"github.com/matrix-org/dendrite/keyserver/storage/shared"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

// A Database is used to store room events and stream offsets.
type Database struct {
	database     cosmosdbutil.Database
	connection   cosmosdbapi.CosmosConnection
	databaseName string
	cosmosConfig cosmosdbapi.CosmosConfig
	serverName   gomatrixserverlib.ServerName
}

func NewDatabase(dbProperties *config.DatabaseOptions) (*shared.Database, error) {
	conn := cosmosdbutil.GetCosmosConnection(&dbProperties.ConnectionString)
	configCosmos := cosmosdbutil.GetCosmosConfig(&dbProperties.ConnectionString)
	result := &Database{
		databaseName: "keyserver",
		connection:   conn,
		cosmosConfig: configCosmos,
	}

	result.database = cosmosdbutil.Database{
		Connection:   conn,
		CosmosConfig: configCosmos,
		DatabaseName: result.databaseName,
	}

	// db, err := sqlutil.Open(dbProperties)
	// if err != nil {
	// 	return nil, err
	// }
	otk, err := NewCosmosDBOneTimeKeysTable(result)
	if err != nil {
		return nil, err
	}
	dk, err := NewCosmosDBDeviceKeysTable(result)
	if err != nil {
		return nil, err
	}
	kc, err := NewCosmosDBKeyChangesTable(result)
	if err != nil {
		return nil, err
	}
	sdl, err := NewCosmosDBStaleDeviceListsTable(result)
	if err != nil {
		return nil, err
	}
	csk, err := NewSqliteCrossSigningKeysTable(result)
	if err != nil {
		return nil, err
	}
	css, err := NewSqliteCrossSigningSigsTable(result)
	if err != nil {
		return nil, err
	}

	writer := cosmosdbutil.NewExclusiveWriterFake()
	storer := cosmosdbutil.PartitionOffsetStatements{}
	if err = storer.Prepare(&result.database, writer, "keyserver"); err != nil {
		return nil, err
	}

	return &shared.Database{
		Writer:                writer,
		OneTimeKeysTable:      otk,
		DeviceKeysTable:       dk,
		KeyChangesTable:       kc,
		StaleDeviceListsTable: sdl,
		CrossSigningKeysTable: csk,
		CrossSigningSigsTable: css,
		PartitionStorer:       &storer,
	}, nil
}
