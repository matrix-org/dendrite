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

	_ "github.com/mattn/go-sqlite3"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/storage/sqlite3/deltas"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// A Database is used to store room events and stream offsets.
type Database struct {
	shared.Database
}

// Open a sqlite database.
func Open(dbProperties *config.DatabaseOptions, cache caching.RoomServerCaches) (*Database, error) {
	var d Database
	var db *sql.DB
	var err error
	if db, err = sqlutil.Open(dbProperties); err != nil {
		return nil, err
	}

	//db.Exec("PRAGMA journal_mode=WAL;")
	//db.Exec("PRAGMA read_uncommitted = true;")

	// FIXME: We are leaking connections somewhere. Setting this to 2 will eventually
	// cause the roomserver to be unresponsive to new events because something will
	// acquire the global mutex and never unlock it because it is waiting for a connection
	// which it will never obtain.
	db.SetMaxOpenConns(20)

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
func (d *Database) prepare(db *sql.DB, cache caching.RoomServerCaches) error {
	var err error
	eventStateKeys, err := NewSqliteEventStateKeysTable(db)
	if err != nil {
		return err
	}
	eventTypes, err := NewSqliteEventTypesTable(db)
	if err != nil {
		return err
	}
	eventJSON, err := NewSqliteEventJSONTable(db)
	if err != nil {
		return err
	}
	events, err := NewSqliteEventsTable(db)
	if err != nil {
		return err
	}
	rooms, err := NewSqliteRoomsTable(db)
	if err != nil {
		return err
	}
	transactions, err := NewSqliteTransactionsTable(db)
	if err != nil {
		return err
	}
	stateBlock, err := NewSqliteStateBlockTable(db)
	if err != nil {
		return err
	}
	stateSnapshot, err := NewSqliteStateSnapshotTable(db)
	if err != nil {
		return err
	}
	prevEvents, err := NewSqlitePrevEventsTable(db)
	if err != nil {
		return err
	}
	roomAliases, err := NewSqliteRoomAliasesTable(db)
	if err != nil {
		return err
	}
	invites, err := NewSqliteInvitesTable(db)
	if err != nil {
		return err
	}
	membership, err := NewSqliteMembershipTable(db)
	if err != nil {
		return err
	}
	published, err := NewSqlitePublishedTable(db)
	if err != nil {
		return err
	}
	redactions, err := NewSqliteRedactionsTable(db)
	if err != nil {
		return err
	}
	d.Database = shared.Database{
		DB:                         db,
		Cache:                      cache,
		Writer:                     sqlutil.NewExclusiveWriter(),
		EventsTable:                events,
		EventTypesTable:            eventTypes,
		EventStateKeysTable:        eventStateKeys,
		EventJSONTable:             eventJSON,
		RoomsTable:                 rooms,
		TransactionsTable:          transactions,
		StateBlockTable:            stateBlock,
		StateSnapshotTable:         stateSnapshot,
		PrevEventsTable:            prevEvents,
		RoomAliasesTable:           roomAliases,
		InvitesTable:               invites,
		MembershipTable:            membership,
		PublishedTable:             published,
		RedactionsTable:            redactions,
		GetLatestEventsForUpdateFn: d.GetLatestEventsForUpdate,
	}
	return nil
}

func (d *Database) SupportsConcurrentRoomInputs() bool {
	// This isn't supported in SQLite mode yet because of issues with
	// database locks.
	// TODO: Look at this again - the problem is probably to do with
	// the membership updaters and latest events updaters.
	return false
}

func (d *Database) GetLatestEventsForUpdate(
	ctx context.Context, roomInfo types.RoomInfo,
) (*shared.LatestEventsUpdater, error) {
	// TODO: Do not use transactions. We should be holding open this transaction but we cannot have
	// multiple write transactions on sqlite. The code will perform additional
	// write transactions independent of this one which will consistently cause
	// 'database is locked' errors. As sqlite doesn't support multi-process on the
	// same DB anyway, and we only execute updates sequentially, the only worries
	// are for rolling back when things go wrong. (atomicity)
	return shared.NewLatestEventsUpdater(ctx, &d.Database, nil, roomInfo)
}

func (d *Database) MembershipUpdater(
	ctx context.Context, roomID, targetUserID string,
	targetLocal bool, roomVersion gomatrixserverlib.RoomVersion,
) (*shared.MembershipUpdater, error) {
	// TODO: Do not use transactions. We should be holding open this transaction but we cannot have
	// multiple write transactions on sqlite. The code will perform additional
	// write transactions independent of this one which will consistently cause
	// 'database is locked' errors. As sqlite doesn't support multi-process on the
	// same DB anyway, and we only execute updates sequentially, the only worries
	// are for rolling back when things go wrong. (atomicity)
	return shared.NewMembershipUpdater(ctx, &d.Database, nil, roomID, targetUserID, targetLocal, roomVersion)
}
