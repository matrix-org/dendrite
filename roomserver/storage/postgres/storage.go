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

	"github.com/matrix-org/dendrite/internal/sqlutil"

	// Import the postgres database driver.
	_ "github.com/lib/pq"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
)

// A Database is used to store room events and stream offsets.
type Database struct {
	shared.Database
}

// Open a postgres database.
// nolint: gocyclo
func Open(dataSourceName string, dbProperties sqlutil.DbProperties) (*Database, error) {
	var d Database
	var db *sql.DB
	var err error
	if db, err = sqlutil.Open("postgres", dataSourceName, dbProperties); err != nil {
		return nil, err
	}
	eventStateKeys, err := NewPostgresEventStateKeysTable(db)
	if err != nil {
		return nil, err
	}
	eventTypes, err := NewPostgresEventTypesTable(db)
	if err != nil {
		return nil, err
	}
	eventJSON, err := NewPostgresEventJSONTable(db)
	if err != nil {
		return nil, err
	}
	events, err := NewPostgresEventsTable(db)
	if err != nil {
		return nil, err
	}
	rooms, err := NewPostgresRoomsTable(db)
	if err != nil {
		return nil, err
	}
	transactions, err := NewPostgresTransactionsTable(db)
	if err != nil {
		return nil, err
	}
	stateBlock, err := NewPostgresStateBlockTable(db)
	if err != nil {
		return nil, err
	}
	stateSnapshot, err := NewPostgresStateSnapshotTable(db)
	if err != nil {
		return nil, err
	}
	roomAliases, err := NewPostgresRoomAliasesTable(db)
	if err != nil {
		return nil, err
	}
	prevEvents, err := NewPostgresPreviousEventsTable(db)
	if err != nil {
		return nil, err
	}
	invites, err := NewPostgresInvitesTable(db)
	if err != nil {
		return nil, err
	}
	membership, err := NewPostgresMembershipTable(db)
	if err != nil {
		return nil, err
	}
	published, err := NewPostgresPublishedTable(db)
	if err != nil {
		return nil, err
	}
	redactions, err := NewPostgresRedactionsTable(db)
	if err != nil {
		return nil, err
	}
	d.Database = shared.Database{
		DB:                  db,
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
	return &d, nil
}
