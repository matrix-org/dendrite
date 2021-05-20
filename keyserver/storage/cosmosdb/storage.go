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
	shared.Database
	connection   cosmosdbapi.CosmosConnection
	databaseName string
	cosmosConfig cosmosdbapi.CosmosConfig
	serverName   gomatrixserverlib.ServerName
}

func NewDatabase(dbProperties *config.DatabaseOptions) (*shared.Database, error) {
	conn := cosmosdbutil.GetCosmosConnection(&dbProperties.ConnectionString)
	config := cosmosdbutil.GetCosmosConfig(&dbProperties.ConnectionString)
	d := &Database{
		databaseName: "keyserver",
		connection:   conn,
		cosmosConfig: config,
	}

	// db, err := sqlutil.Open(dbProperties)
	// if err != nil {
	// 	return nil, err
	// }
	otk, err := NewCosmosDBOneTimeKeysTable(d)
	if err != nil {
		return nil, err
	}
	dk, err := NewCosmosDBDeviceKeysTable(d)
	if err != nil {
		return nil, err
	}
	kc, err := NewCosmosDBKeyChangesTable(d)
	if err != nil {
		return nil, err
	}
	sdl, err := NewCosmosDBStaleDeviceListsTable(d)
	if err != nil {
		return nil, err
	}
	return &shared.Database{
		Writer:                cosmosdbutil.NewExclusiveWriterFake(),
		OneTimeKeysTable:      otk,
		DeviceKeysTable:       dk,
		KeyChangesTable:       kc,
		StaleDeviceListsTable: sdl,
	}, nil
}
