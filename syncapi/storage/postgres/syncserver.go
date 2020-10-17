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

package postgres

import (
	"database/sql"

	// Import the postgres database driver.
	_ "github.com/lib/pq"

	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/postgres/deltas"
	"github.com/matrix-org/dendrite/syncapi/storage/shared"
)

// SyncServerDatasource represents a sync server datasource which manages
// both the database for PDUs and caches for EDUs.
type SyncServerDatasource struct {
	shared.Database
	db     *sql.DB
	writer sqlutil.Writer
	sqlutil.PartitionOffsetStatements
}

const createSequence = "CREATE SEQUENCE IF NOT EXISTS syncapi_stream_id;"

// NewDatabase creates a new sync server database
func NewDatabase(dbProperties *config.DatabaseOptions) (*SyncServerDatasource, error) {
	var d SyncServerDatasource
	var err error
	if d.db, err = sqlutil.Open(dbProperties); err != nil {
		return nil, err
	}
	d.writer = sqlutil.NewDummyWriter()

	// Make sure the required, for most tables, exists.
	if _, err = d.db.Exec(createSequence); err != nil {
		return nil, err
	}

	// Create tables before executing migrations so we don't fail if the table is missing,
	// and THEN prepare statements so we don't fail due to referencing new columns
	r := receiptStatements{}
	if err = r.execSchema(d.db); err != nil {
		return nil, err
	}
	m := sqlutil.NewMigrations()
	deltas.LoadCreateReceiptTable(m)
	if err = m.RunDeltas(d.db, dbProperties); err != nil {
		return nil, err
	}
	if err = d.prepare(); err != nil {
		return nil, err
	}

	return &d, nil
}

func (d *SyncServerDatasource) prepare() (err error) {
	if err = d.PartitionOffsetStatements.Prepare(d.db, d.writer, "syncapi"); err != nil {
		return err
	}
	accountData, err := NewPostgresAccountDataTable(d.db)
	if err != nil {
		return err
	}
	events, err := NewPostgresEventsTable(d.db)
	if err != nil {
		return err
	}
	currState, err := NewPostgresCurrentRoomStateTable(d.db)
	if err != nil {
		return err
	}
	invites, err := NewPostgresInvitesTable(d.db)
	if err != nil {
		return err
	}
	peeks, err := NewPostgresPeeksTable(d.db)
	if err != nil {
		return err
	}
	topology, err := NewPostgresTopologyTable(d.db)
	if err != nil {
		return err
	}
	backwardExtremities, err := NewPostgresBackwardsExtremitiesTable(d.db)
	if err != nil {
		return err
	}
	sendToDevice, err := NewPostgresSendToDeviceTable(d.db)
	if err != nil {
		return err
	}
	filter, err := NewPostgresFilterTable(d.db)
	if err != nil {
		return err
	}
	receipts, err := NewPostgresReceiptsTable(d.db)
	if err != nil {
		return err
	}
	d.Database = shared.Database{
		DB:                  d.db,
		Writer:              d.writer,
		Invites:             invites,
		Peeks:               peeks,
		AccountData:         accountData,
		OutputEvents:        events,
		Topology:            topology,
		CurrentRoomState:    currState,
		BackwardExtremities: backwardExtremities,
		Filter:              filter,
		SendToDevice:        sendToDevice,
		Receipts:            receipts,
		EDUCache:            cache.New(),
	}
	return nil
}
