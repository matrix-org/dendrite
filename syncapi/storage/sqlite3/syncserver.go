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
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/storage/shared"
	"github.com/matrix-org/dendrite/syncapi/storage/sqlite3/deltas"
)

// SyncServerDatasource represents a sync server datasource which manages
// both the database for PDUs and caches for EDUs.
type SyncServerDatasource struct {
	shared.Database
	db       *sql.DB
	writer   sqlutil.Writer
	streamID StreamIDStatements
}

// NewDatabase creates a new sync server database
// nolint: gocyclo
func NewDatabase(ctx context.Context, conMan *sqlutil.Connections, dbProperties *config.DatabaseOptions) (*SyncServerDatasource, error) {
	var d SyncServerDatasource
	var err error

	if d.db, d.writer, err = conMan.Connection(dbProperties); err != nil {
		return nil, err
	}
	if err = d.prepare(ctx); err != nil {
		return nil, err
	}
	return &d, nil
}

func (d *SyncServerDatasource) NewDatabaseSnapshot(ctx context.Context) (*shared.DatabaseTransaction, error) {
	return &shared.DatabaseTransaction{
		Database: &d.Database,
		// not setting a transaction because SQLite doesn't support it
	}, nil
}

func (d *SyncServerDatasource) NewDatabaseTransaction(ctx context.Context) (*shared.DatabaseTransaction, error) {
	return &shared.DatabaseTransaction{
		Database: &d.Database,
		// not setting a transaction because SQLite doesn't support it
	}, nil
}

func (d *SyncServerDatasource) prepare(ctx context.Context) (err error) {
	if err = d.streamID.Prepare(d.db); err != nil {
		return err
	}
	accountData, err := NewSqliteAccountDataTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	events, err := NewSqliteEventsTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	roomState, err := NewSqliteCurrentRoomStateTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	invites, err := NewSqliteInvitesTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	peeks, err := NewSqlitePeeksTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	topology, err := NewSqliteTopologyTable(d.db)
	if err != nil {
		return err
	}
	bwExtrem, err := NewSqliteBackwardsExtremitiesTable(d.db)
	if err != nil {
		return err
	}
	sendToDevice, err := NewSqliteSendToDeviceTable(d.db)
	if err != nil {
		return err
	}
	filter, err := NewSqliteFilterTable(d.db)
	if err != nil {
		return err
	}
	receipts, err := NewSqliteReceiptsTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	memberships, err := NewSqliteMembershipsTable(d.db)
	if err != nil {
		return err
	}
	notificationData, err := NewSqliteNotificationDataTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	ignores, err := NewSqliteIgnoresTable(d.db)
	if err != nil {
		return err
	}
	presence, err := NewSqlitePresenceTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	relations, err := NewSqliteRelationsTable(d.db, &d.streamID)
	if err != nil {
		return err
	}

	// apply migrations which need multiple tables
	m := sqlutil.NewMigrator(d.db)
	m.AddMigrations(
		sqlutil.Migration{
			Version: "syncapi: set history visibility for existing events",
			Up:      deltas.UpSetHistoryVisibility, // Requires current_room_state and output_room_events to be created.
		},
	)
	err = m.Up(ctx)
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
		BackwardExtremities: bwExtrem,
		CurrentRoomState:    roomState,
		Topology:            topology,
		Filter:              filter,
		SendToDevice:        sendToDevice,
		Receipts:            receipts,
		Memberships:         memberships,
		NotificationData:    notificationData,
		Ignores:             ignores,
		Presence:            presence,
		Relations:           relations,
	}
	return nil
}
