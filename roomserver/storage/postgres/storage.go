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

	"github.com/lib/pq"
	// Import the postgres database driver.
	_ "github.com/lib/pq"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/postgres/deltas"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
)

// A Database is used to store room events and stream offsets.
type Database struct {
	shared.Database
}

// Open a postgres database.
func Open(base *base.BaseDendrite, dbProperties *config.DatabaseOptions, cache caching.RoomServerCaches) (*Database, error) {
	var d Database
	var err error
	db, writer, err := base.DatabaseConnection(dbProperties, sqlutil.NewDummyWriter())
	if err != nil {
		return nil, fmt.Errorf("sqlutil.Open: %w", err)
	}

	// Create the tables.
	if err = d.create(db); err != nil {
		return nil, err
	}

	// Special case, since this migration uses several tables, so it needs to
	// be sure that all tables are created first.
	// TODO: Remove when we are sure we are not having goose artefacts in the db
	// This forces an error, which indicates the migration is already applied, since the
	// column event_nid was removed from the table
	var eventNID int
	err = db.QueryRow("SELECT event_nid FROM roomserver_state_block LIMIT 1;").Scan(&eventNID)
	if err == nil {
		m := sqlutil.NewMigrator(db)
		m.AddMigrations(sqlutil.Migration{
			Version: "roomserver: state blocks refactor",
			Up:      deltas.UpStateBlocksRefactor,
		})
		if err = m.Up(base.Context()); err != nil {
			return nil, err
		}
	} else {
		switch e := err.(type) {
		case *pq.Error:
			// ignore undefined_column (42703) errors, as this is expected at this point
			if e.Code != "42703" {
				return nil, err
			}
		default:
			return nil, err
		}
	}

	// Then prepare the statements. Now that the migrations have run, any columns referred
	// to in the database code should now exist.
	if err = d.prepare(db, writer, cache); err != nil {
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
	if err := CreateRoomsTable(db); err != nil {
		return err
	}
	if err := CreateStateBlockTable(db); err != nil {
		return err
	}
	if err := CreateStateSnapshotTable(db); err != nil {
		return err
	}
	if err := CreatePrevEventsTable(db); err != nil {
		return err
	}
	if err := CreateRoomAliasesTable(db); err != nil {
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
	rooms, err := PrepareRoomsTable(db)
	if err != nil {
		return err
	}
	stateBlock, err := PrepareStateBlockTable(db)
	if err != nil {
		return err
	}
	stateSnapshot, err := PrepareStateSnapshotTable(db)
	if err != nil {
		return err
	}
	prevEvents, err := PreparePrevEventsTable(db)
	if err != nil {
		return err
	}
	roomAliases, err := PrepareRoomAliasesTable(db)
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
		EventTypesTable:     eventTypes,
		EventStateKeysTable: eventStateKeys,
		EventJSONTable:      eventJSON,
		EventsTable:         events,
		RoomsTable:          rooms,
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
