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
		return nil, err
	}

	// Create tables before executing migrations so we don't fail if the table is missing,
	// and THEN prepare statements so we don't fail due to referencing new columns
	ms := membershipStatements{}
	if err := ms.execSchema(db); err != nil {
		return nil, err
	}
	m := sqlutil.NewMigrations()
	deltas.LoadAddForgottenColumn(m)
	if err := m.RunDeltas(db, dbProperties); err != nil {
		return nil, err
	}
	if err := d.prepare(db, cache); err != nil {
		return nil, err
	}

	return &d, nil
}

// nolint: gocyclo
func (d *Database) prepare(db *sql.DB, cache caching.RoomServerCaches) (err error) {
	eventStateKeys, err := NewPostgresEventStateKeysTable(db)
	if err != nil {
		return err
	}
	eventTypes, err := NewPostgresEventTypesTable(db)
	if err != nil {
		return err
	}
	eventJSON, err := NewPostgresEventJSONTable(db)
	if err != nil {
		return err
	}
	events, err := NewPostgresEventsTable(db)
	if err != nil {
		return err
	}
	rooms, err := NewPostgresRoomsTable(db)
	if err != nil {
		return err
	}
	transactions, err := NewPostgresTransactionsTable(db)
	if err != nil {
		return err
	}
	stateBlock, err := NewPostgresStateBlockTable(db)
	if err != nil {
		return err
	}
	stateSnapshot, err := NewPostgresStateSnapshotTable(db)
	if err != nil {
		return err
	}
	roomAliases, err := NewPostgresRoomAliasesTable(db)
	if err != nil {
		return err
	}
	prevEvents, err := NewPostgresPreviousEventsTable(db)
	if err != nil {
		return err
	}
	invites, err := NewPostgresInvitesTable(db)
	if err != nil {
		return err
	}
	membership, err := NewPostgresMembershipTable(db)
	if err != nil {
		return err
	}
	published, err := NewPostgresPublishedTable(db)
	if err != nil {
		return err
	}
	redactions, err := NewPostgresRedactionsTable(db)
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
