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
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implie
// See the License for the specific language governing permissions and
// limitations under the License.

package postgres

import (
	"database/sql"
	"fmt"

	// Import the postgres database driver.
	_ "github.com/lib/pq"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/postgres/deltas"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/setup/config"
)

// A Database is used to store room events and stream offsets.
type Database struct {
	shared.Database
}

// Open a postgres database.
func Open(dbProperties *config.DatabaseOptions, cache caching.RoomServerCaches) (*Database, error) {
	var d Database
	var db *sql.DB
	var err error
	if db, err = sqlutil.Open(dbProperties); err != nil {
		return nil, fmt.Errorf("sqlutil.Open: %w", err)
	}

	// Create the tables.
	if err := d.create(db); err != nil {
		return nil, err
	}

	// Then execute the migrations. By this point the tables are created with the latest
	// schemas.
	m := sqlutil.NewMigrations()
	deltas.LoadAddForgottenColumn(m)
	deltas.LoadStateBlocksRefactor(m)
	if err := m.RunDeltas(db, dbProperties); err != nil {
		return nil, err
	}

	// Then prepare the statements. Now that the migrations have run, any columns referred
	// to in the database code should now exist.
	if err := d.prepare(db, cache); err != nil {
		return nil, err
	}

	return &d, nil
}

func (d *Database) create(db *sql.DB) error {
	if err := createEventStateKeysTable(db); err != nil {
		return err
	}
	if err := createEventTypesTable(db); err != nil {
		return err
	}
	if err := createEventJSONTable(db); err != nil {
		return err
	}
	if err := createEventsTable(db); err != nil {
		return err
	}
	if err := createRoomsTable(db); err != nil {
		return err
	}
	if err := createTransactionsTable(db); err != nil {
		return err
	}
	if err := createStateBlockTable(db); err != nil {
		return err
	}
	if err := createStateSnapshotTable(db); err != nil {
		return err
	}
	if err := createPrevEventsTable(db); err != nil {
		return err
	}
	if err := createRoomAliasesTable(db); err != nil {
		return err
	}
	if err := createInvitesTable(db); err != nil {
		return err
	}
	if err := createMembershipTable(db); err != nil {
		return err
	}
	if err := createPublishedTable(db); err != nil {
		return err
	}
	if err := createRedactionsTable(db); err != nil {
		return err
	}

	return nil
}

func (d *Database) prepare(db *sql.DB, cache caching.RoomServerCaches) error {
	eventStateKeys, err := prepareEventStateKeysTable(db)
	if err != nil {
		return err
	}
	eventTypes, err := prepareEventTypesTable(db)
	if err != nil {
		return err
	}
	eventJSON, err := prepareEventJSONTable(db)
	if err != nil {
		return err
	}
	events, err := prepareEventsTable(db)
	if err != nil {
		return err
	}
	rooms, err := prepareRoomsTable(db)
	if err != nil {
		return err
	}
	transactions, err := prepareTransactionsTable(db)
	if err != nil {
		return err
	}
	stateBlock, err := prepareStateBlockTable(db)
	if err != nil {
		return err
	}
	stateSnapshot, err := prepareStateSnapshotTable(db)
	if err != nil {
		return err
	}
	prevEvents, err := preparePrevEventsTable(db)
	if err != nil {
		return err
	}
	roomAliases, err := prepareRoomAliasesTable(db)
	if err != nil {
		return err
	}
	invites, err := prepareInvitesTable(db)
	if err != nil {
		return err
	}
	membership, err := prepareMembershipTable(db)
	if err != nil {
		return err
	}
	published, err := preparePublishedTable(db)
	if err != nil {
		return err
	}
	redactions, err := prepareRedactionsTable(db)
	if err != nil {
		return err
	}
	d.Database = shared.Database{
		DB:                  db,
		Cache:               cache,
		Writer:              sqlutil.NewDummyWriter(),
		EventTypesTable:     eventTypes,
		EventStateKeysTable: eventStateKeys,
		EventJSONTable:      eventJSON,
		EventsTable:         events,
		RoomsTable:          rooms,
		TransactionsTable:   transactions,
		StateBlockTable:     stateBlock,
		StateSnapshotTable:  stateSnapshot,
		PrevEventsTable:     prevEvents,
		RoomAliasesTable:    roomAliases,
		InvitesTable:        invites,
		MembershipTable:     membership,
		PublishedTable:      published,
		RedactionsTable:     redactions,
	}
	return nil
}
