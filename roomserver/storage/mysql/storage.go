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

package mysql

import (
	"database/sql"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"

	// Import the mysql database driver.
	_ "github.com/go-sql-driver/mysql"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
)

// A Database is used to store room events and stream offsets.
type Database struct {
	shared.Database
}

// Open a mysql database.
// nolint: gocyclo
func Open(dataSourceName string, dbProperties internal.DbProperties) (*Database, error) {
	var d Database
	var db *sql.DB
	var err error
	if db, err = sqlutil.Open("mysql", dataSourceName, dbProperties); err != nil {
		return nil, err
	}
	eventStateKeys, err := NewMysqlEventStateKeysTable(db)
	if err != nil {
		return nil, err
	}
	eventTypes, err := NewMysqlEventTypesTable(db)
	if err != nil {
		return nil, err
	}
	eventJSON, err := NewMysqlEventJSONTable(db)
	if err != nil {
		return nil, err
	}
	events, err := NewMysqlEventsTable(db)
	if err != nil {
		return nil, err
	}
	rooms, err := NewMysqlRoomsTable(db)
	if err != nil {
		return nil, err
	}
	transactions, err := NewMysqlTransactionsTable(db)
	if err != nil {
		return nil, err
	}
	stateBlock, err := NewMysqlStateBlockTable(db)
	if err != nil {
		return nil, err
	}
	stateSnapshot, err := NewMysqlStateSnapshotTable(db)
	if err != nil {
		return nil, err
	}
	roomAliases, err := NewMysqlRoomAliasesTable(db)
	if err != nil {
		return nil, err
	}
	prevEvents, err := NewMysqlPreviousEventsTable(db)
	if err != nil {
		return nil, err
	}
	invites, err := NewMysqlInvitesTable(db)
	if err != nil {
		return nil, err
	}
	membership, err := NewMysqlMembershipTable(db)
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
	}
	return &d, nil
}
