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
	"fmt"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/storage/sqlite3/deltas"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

// A Database is used to store room events and stream offsets.
type Database struct {
	shared.Database
}

// Open a sqlite database.
func Open(base *base.BaseDendrite, dbProperties *config.DatabaseOptions, cache caching.RoomServerCaches) (*Database, error) {
	var d Database
	var err error
	db, writer, err := base.DatabaseConnection(dbProperties, sqlutil.NewExclusiveWriter())
	if err != nil {
		return nil, fmt.Errorf("sqlutil.Open: %w", err)
	}

	//db.Exec("PRAGMA journal_mode=WAL;")
	//db.Exec("PRAGMA read_uncommitted = true;")

	// FIXME: We are leaking connections somewhere. Setting this to 2 will eventually
	// cause the roomserver to be unresponsive to new events because something will
	// acquire the global mutex and never unlock it because it is waiting for a connection
	// which it will never obtain.
	// db.SetMaxOpenConns(20)

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
	if err := d.prepare(db, writer, cache); err != nil {
		return nil, err
	}

	return &d, nil
}

func (d *Database) create(db *sql.DB) error {
	if err := CreateEventStateKeysTable(db); err != nil {
		return err
	}
	if err := CreateEventTypesTable(db); err != nil {
		return err
	}
	if err := CreateEventJSONTable(db); err != nil {
		return err
	}
	if err := CreateEventsTable(db); err != nil {
		return err
	}
	if err := createRoomsTable(db); err != nil {
		return err
	}
	if err := createStateBlockTable(db); err != nil {
		return err
	}
	if err := createStateSnapshotTable(db); err != nil {
		return err
	}
	if err := CreatePrevEventsTable(db); err != nil {
		return err
	}
	if err := createRoomAliasesTable(db); err != nil {
		return err
	}
	if err := CreateInvitesTable(db); err != nil {
		return err
	}
	if err := CreateMembershipTable(db); err != nil {
		return err
	}
	if err := CreatePublishedTable(db); err != nil {
		return err
	}
	if err := CreateRedactionsTable(db); err != nil {
		return err
	}

	return nil
}

func (d *Database) prepare(db *sql.DB, writer sqlutil.Writer, cache caching.RoomServerCaches) error {
	eventStateKeys, err := PrepareEventStateKeysTable(db)
	if err != nil {
		return err
	}
	eventTypes, err := PrepareEventTypesTable(db)
	if err != nil {
		return err
	}
	eventJSON, err := PrepareEventJSONTable(db)
	if err != nil {
		return err
	}
	events, err := PrepareEventsTable(db)
	if err != nil {
		return err
	}
	rooms, err := prepareRoomsTable(db)
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
	prevEvents, err := PreparePrevEventsTable(db)
	if err != nil {
		return err
	}
	roomAliases, err := prepareRoomAliasesTable(db)
	if err != nil {
		return err
	}
	invites, err := PrepareInvitesTable(db)
	if err != nil {
		return err
	}
	membership, err := PrepareMembershipTable(db)
	if err != nil {
		return err
	}
	published, err := PreparePublishedTable(db)
	if err != nil {
		return err
	}
	redactions, err := PrepareRedactionsTable(db)
	if err != nil {
		return err
	}
	d.Database = shared.Database{
		DB:                  db,
		Cache:               cache,
		Writer:              writer,
		EventsTable:         events,
		EventTypesTable:     eventTypes,
		EventStateKeysTable: eventStateKeys,
		EventJSONTable:      eventJSON,
		RoomsTable:          rooms,
		StateBlockTable:     stateBlock,
		StateSnapshotTable:  stateSnapshot,
		PrevEventsTable:     prevEvents,
		RoomAliasesTable:    roomAliases,
		InvitesTable:        invites,
		MembershipTable:     membership,
		PublishedTable:      published,
		RedactionsTable:     redactions,
		GetRoomUpdaterFn:    d.GetRoomUpdater,
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

func (d *Database) GetRoomUpdater(
	ctx context.Context, roomInfo *types.RoomInfo,
) (*shared.RoomUpdater, error) {
	// TODO: Do not use transactions. We should be holding open this transaction but we cannot have
	// multiple write transactions on sqlite. The code will perform additional
	// write transactions independent of this one which will consistently cause
	// 'database is locked' errors. As sqlite doesn't support multi-process on the
	// same DB anyway, and we only execute updates sequentially, the only worries
	// are for rolling back when things go wrong. (atomicity)
	return shared.NewRoomUpdater(ctx, &d.Database, nil, roomInfo)
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
