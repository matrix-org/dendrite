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

	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	_ "github.com/mattn/go-sqlite3"
)

// A Database is used to store room events and stream offsets.
type Database struct {
	shared.Database
	events         tables.Events
	eventJSON      tables.EventJSON
	eventTypes     tables.EventTypes
	eventStateKeys tables.EventStateKeys
	rooms          tables.Rooms
	transactions   tables.Transactions
	prevEvents     tables.PreviousEvents
	invites        tables.Invites
	membership     tables.Membership
	db             *sql.DB
}

// Open a sqlite database.
// nolint: gocyclo
func Open(dbProperties *config.DatabaseOptions) (*Database, error) {
	var d Database
	var err error
	if d.db, err = sqlutil.Open(dbProperties); err != nil {
		return nil, err
	}
	//d.db.Exec("PRAGMA journal_mode=WAL;")
	//d.db.Exec("PRAGMA read_uncommitted = true;")

	// FIXME: We are leaking connections somewhere. Setting this to 2 will eventually
	// cause the roomserver to be unresponsive to new events because something will
	// acquire the global mutex and never unlock it because it is waiting for a connection
	// which it will never obtain.
	d.db.SetMaxOpenConns(20)

	d.eventStateKeys, err = NewSqliteEventStateKeysTable(d.db)
	if err != nil {
		return nil, err
	}
	d.eventTypes, err = NewSqliteEventTypesTable(d.db)
	if err != nil {
		return nil, err
	}
	d.eventJSON, err = NewSqliteEventJSONTable(d.db)
	if err != nil {
		return nil, err
	}
	d.events, err = NewSqliteEventsTable(d.db)
	if err != nil {
		return nil, err
	}
	d.rooms, err = NewSqliteRoomsTable(d.db)
	if err != nil {
		return nil, err
	}
	d.transactions, err = NewSqliteTransactionsTable(d.db)
	if err != nil {
		return nil, err
	}
	stateBlock, err := NewSqliteStateBlockTable(d.db)
	if err != nil {
		return nil, err
	}
	stateSnapshot, err := NewSqliteStateSnapshotTable(d.db)
	if err != nil {
		return nil, err
	}
	d.prevEvents, err = NewSqlitePrevEventsTable(d.db)
	if err != nil {
		return nil, err
	}
	roomAliases, err := NewSqliteRoomAliasesTable(d.db)
	if err != nil {
		return nil, err
	}
	d.invites, err = NewSqliteInvitesTable(d.db)
	if err != nil {
		return nil, err
	}
	d.membership, err = NewSqliteMembershipTable(d.db)
	if err != nil {
		return nil, err
	}
	published, err := NewSqlitePublishedTable(d.db)
	if err != nil {
		return nil, err
	}
	redactions, err := NewSqliteRedactionsTable(d.db)
	if err != nil {
		return nil, err
	}
	d.Database = shared.Database{
		DB:                  d.db,
		EventsTable:         d.events,
		EventTypesTable:     d.eventTypes,
		EventStateKeysTable: d.eventStateKeys,
		EventJSONTable:      d.eventJSON,
		RoomsTable:          d.rooms,
		TransactionsTable:   d.transactions,
		StateBlockTable:     stateBlock,
		StateSnapshotTable:  stateSnapshot,
		PrevEventsTable:     d.prevEvents,
		RoomAliasesTable:    roomAliases,
		InvitesTable:        d.invites,
		MembershipTable:     d.membership,
		PublishedTable:      published,
		RedactionsTable:     redactions,
	}
	return &d, nil
}

func (d *Database) GetLatestEventsForUpdate(
	ctx context.Context, roomNID types.RoomNID,
) (types.RoomRecentEventsUpdater, error) {
	// TODO: Do not use transactions. We should be holding open this transaction but we cannot have
	// multiple write transactions on sqlite. The code will perform additional
	// write transactions independent of this one which will consistently cause
	// 'database is locked' errors. As sqlite doesn't support multi-process on the
	// same DB anyway, and we only execute updates sequentially, the only worries
	// are for rolling back when things go wrong. (atomicity)
	return shared.NewRoomRecentEventsUpdater(&d.Database, ctx, roomNID, false)
}

func (d *Database) MembershipUpdater(
	ctx context.Context, roomID, targetUserID string,
	targetLocal bool, roomVersion gomatrixserverlib.RoomVersion,
) (updater types.MembershipUpdater, err error) {
	// TODO: Do not use transactions. We should be holding open this transaction but we cannot have
	// multiple write transactions on sqlite. The code will perform additional
	// write transactions independent of this one which will consistently cause
	// 'database is locked' errors. As sqlite doesn't support multi-process on the
	// same DB anyway, and we only execute updates sequentially, the only worries
	// are for rolling back when things go wrong. (atomicity)
	return shared.NewMembershipUpdater(ctx, &d.Database, roomID, targetUserID, targetLocal, roomVersion, false)
}
