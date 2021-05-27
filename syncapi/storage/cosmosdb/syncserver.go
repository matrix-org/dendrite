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

	// Import the sqlite3 package
	// _ "github.com/mattn/go-sqlite3"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/storage/shared"
)

// SyncServerDatasource represents a sync server datasource which manages
// both the database for PDUs and caches for EDUs.
type SyncServerDatasource struct {
	shared.Database
	// db     *sql.DB
	writer   cosmosdbutil.Writer
	database cosmosdbutil.Database
	cosmosdbutil.PartitionOffsetStatements
	streamID     streamIDStatements
	connection   cosmosdbapi.CosmosConnection
	databaseName string
	cosmosConfig cosmosdbapi.CosmosConfig
}

// NewDatabase creates a new sync server database
// nolint: gocyclo
func NewDatabase(dbProperties *config.DatabaseOptions) (*SyncServerDatasource, error) {
	conn := cosmosdbutil.GetCosmosConnection(&dbProperties.ConnectionString)
	configCosmos := cosmosdbutil.GetCosmosConfig(&dbProperties.ConnectionString)
	var d SyncServerDatasource
	d.writer = cosmosdbutil.NewExclusiveWriterFake()
	if err := d.prepare(dbProperties); err != nil {
		return nil, err
	}
	d.connection = conn
	d.cosmosConfig = configCosmos
	d.databaseName = "syncapi"
	d.database = cosmosdbutil.Database{
		Connection:   conn,
		CosmosConfig: configCosmos,
		DatabaseName: d.databaseName,
	}
	return &d, nil
}

func (d *SyncServerDatasource) prepare(dbProperties *config.DatabaseOptions) (err error) {
	if err = d.PartitionOffsetStatements.Prepare(&d.database, d.writer, "syncapi"); err != nil {
		return err
	}
	if err = d.streamID.prepare(d); err != nil {
		return err
	}
	accountData, err := NewCosmosDBAccountDataTable(d, &d.streamID)
	if err != nil {
		return err
	}
	events, err := NewCosmosDBEventsTable(d, &d.streamID)
	if err != nil {
		return err
	}
	roomState, err := NewCosmosDBCurrentRoomStateTable(d, &d.streamID)
	if err != nil {
		return err
	}
	invites, err := NewCosmosDBInvitesTable(d, &d.streamID)
	if err != nil {
		return err
	}
	peeks, err := NewCosmosDBPeeksTable(d, &d.streamID)
	if err != nil {
		return err
	}
	topology, err := NewCosmosDBTopologyTable(d)
	if err != nil {
		return err
	}
	bwExtrem, err := NewCosmosDBBackwardsExtremitiesTable(d)
	if err != nil {
		return err
	}
	sendToDevice, err := NewCosmosDBSendToDeviceTable(d)
	if err != nil {
		return err
	}
	filter, err := NewCosmosDBFilterTable(d)
	if err != nil {
		return err
	}
	receipts, err := NewCosmosDBReceiptsTable(d, &d.streamID)
	if err != nil {
		return err
	}
	memberships, err := NewCosmosDBMembershipsTable(d)
	if err != nil {
		return err
	}
	d.Database = shared.Database{
		DB:                  nil,
		Writer:              d.writer,
		Invites:             invites,
		Peeks:               peeks,
		AccountData:         accountData,
		OutputEvents:        events,
		BackwardExtremities: bwExtrem,
		CurrentRoomState:    roomState,
		Topology:            topology,
		Filter:              filter,
		SendToDevice:        sendToDevice,
		Receipts:            receipts,
		Memberships:         memberships,
	}
	return nil
}
