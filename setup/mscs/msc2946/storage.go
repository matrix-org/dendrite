// Copyright 2021 The Matrix.org Foundation C.I.C.
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

package msc2946

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

var (
	relTypes = map[string]int{
		ConstSpaceChildEventType:  1,
		ConstSpaceParentEventType: 2,
	}
)

type Database interface {
	// StoreReference persists a child or parent space mapping.
	StoreReference(ctx context.Context, he *gomatrixserverlib.HeaderedEvent) error
	// References returns all events which have the given roomID as a parent or child space.
	References(ctx context.Context, roomID string) ([]*gomatrixserverlib.HeaderedEvent, error)
}

type DB struct {
	db              *sql.DB
	writer          sqlutil.Writer
	insertEdgeStmt  *sql.Stmt
	selectEdgesStmt *sql.Stmt
}

// NewDatabase loads the database for msc2836
func NewDatabase(dbOpts *config.DatabaseOptions) (Database, error) {
	if dbOpts.ConnectionString.IsPostgres() {
		return newPostgresDatabase(dbOpts)
	}
	return newSQLiteDatabase(dbOpts)
}

func newPostgresDatabase(dbOpts *config.DatabaseOptions) (Database, error) {
	d := DB{
		writer: sqlutil.NewDummyWriter(),
	}
	var err error
	if d.db, err = sqlutil.Open(dbOpts); err != nil {
		return nil, err
	}
	_, err = d.db.Exec(`
	CREATE TABLE IF NOT EXISTS msc2946_edges (
		room_version TEXT NOT NULL,
		-- the room ID of the event, the source of the arrow
		source_room_id TEXT NOT NULL,
		-- the target room ID, the arrow destination
		dest_room_id TEXT NOT NULL,
		-- the kind of relation, either child or parent (1,2)
		rel_type SMALLINT NOT NULL,
		event_json TEXT NOT NULL,
		CONSTRAINT msc2946_edges_uniq UNIQUE (source_room_id, dest_room_id, rel_type)
	);
	`)
	if err != nil {
		return nil, err
	}
	if d.insertEdgeStmt, err = d.db.Prepare(`
		INSERT INTO msc2946_edges(room_version, source_room_id, dest_room_id, rel_type, event_json)
		VALUES($1, $2, $3, $4, $5)
		ON CONFLICT ON CONSTRAINT msc2946_edges_uniq DO UPDATE SET event_json = $5
	`); err != nil {
		return nil, err
	}
	if d.selectEdgesStmt, err = d.db.Prepare(`
		SELECT room_version, event_json FROM msc2946_edges
		WHERE source_room_id = $1 OR dest_room_id = $2
	`); err != nil {
		return nil, err
	}
	return &d, err
}

func newSQLiteDatabase(dbOpts *config.DatabaseOptions) (Database, error) {
	d := DB{
		writer: sqlutil.NewExclusiveWriter(),
	}
	var err error
	if d.db, err = sqlutil.Open(dbOpts); err != nil {
		return nil, err
	}
	_, err = d.db.Exec(`
	CREATE TABLE IF NOT EXISTS msc2946_edges (
		room_version TEXT NOT NULL,
		-- the room ID of the event, the source of the arrow
		source_room_id TEXT NOT NULL,
		-- the target room ID, the arrow destination
		dest_room_id TEXT NOT NULL,
		-- the kind of relation, either child or parent (1,2)
		rel_type SMALLINT NOT NULL,
		event_json TEXT NOT NULL,
		UNIQUE (source_room_id, dest_room_id, rel_type)
	);
	`)
	if err != nil {
		return nil, err
	}
	if d.insertEdgeStmt, err = d.db.Prepare(`
		INSERT INTO msc2946_edges(room_version, source_room_id, dest_room_id, rel_type, event_json)
		VALUES($1, $2, $3, $4, $5)
		ON CONFLICT (source_room_id, dest_room_id, rel_type) DO UPDATE SET event_json = $5
	`); err != nil {
		return nil, err
	}
	if d.selectEdgesStmt, err = d.db.Prepare(`
		SELECT room_version, event_json FROM msc2946_edges
		WHERE source_room_id = $1 OR dest_room_id = $2
	`); err != nil {
		return nil, err
	}
	return &d, err
}

func (d *DB) StoreReference(ctx context.Context, he *gomatrixserverlib.HeaderedEvent) error {
	target := SpaceTarget(he)
	if target == "" {
		return nil // malformed event
	}
	relType := relTypes[he.Type()]
	_, err := d.insertEdgeStmt.ExecContext(ctx, he.RoomVersion, he.RoomID(), target, relType, he.JSON())
	return err
}

func (d *DB) References(ctx context.Context, roomID string) ([]*gomatrixserverlib.HeaderedEvent, error) {
	rows, err := d.selectEdgesStmt.QueryContext(ctx, roomID, roomID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "failed to close References")
	refs := make([]*gomatrixserverlib.HeaderedEvent, 0)
	for rows.Next() {
		var roomVer string
		var jsonBytes []byte
		if err := rows.Scan(&roomVer, &jsonBytes); err != nil {
			return nil, err
		}
		ev, err := gomatrixserverlib.NewEventFromTrustedJSON(jsonBytes, false, gomatrixserverlib.RoomVersion(roomVer))
		if err != nil {
			return nil, err
		}
		he := ev.Headered(gomatrixserverlib.RoomVersion(roomVer))
		refs = append(refs, he)
	}
	return refs, nil
}

// SpaceTarget returns the destination room ID for the space event. This is either a child or a parent
// depending on the event type.
func SpaceTarget(he *gomatrixserverlib.HeaderedEvent) string {
	if he.StateKey() == nil {
		return "" // no-op
	}
	switch he.Type() {
	case ConstSpaceParentEventType:
		return *he.StateKey()
	case ConstSpaceChildEventType:
		return *he.StateKey()
	}
	return ""
}
