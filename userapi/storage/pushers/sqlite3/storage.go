// Copyright 2017 Vector Creations Ltd
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
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"

	_ "github.com/mattn/go-sqlite3"
)

// Database represents a pusher database.
type Database struct {
	db      *sql.DB
	writer  sqlutil.Writer
	pushers pushersStatements
}

// NewDatabase creates a new pusher database
func NewDatabase(dbProperties *config.DatabaseOptions, serverName gomatrixserverlib.ServerName) (*Database, error) {
	db, err := sqlutil.Open(dbProperties)
	if err != nil {
		return nil, err
	}
	writer := sqlutil.NewExclusiveWriter()
	d := pushersStatements{}

	// Create tables before executing migrations so we don't fail if the table is missing,
	// and THEN prepare statements so we don't fail due to referencing new columns
	if err = d.execSchema(db); err != nil {
		return nil, err
	}
	m := sqlutil.NewMigrations()
	if err = m.RunDeltas(db, dbProperties); err != nil {
		return nil, err
	}
	if err = d.prepare(db, writer, serverName); err != nil {
		return nil, err
	}
	return &Database{db, writer, d}, nil
}

// GetPushersByLocalpart returns the pushers matching the given localpart.
func (d *Database) GetPushersByLocalpart(
	ctx context.Context, localpart string,
) ([]api.Pusher, error) {
	return d.pushers.selectPushersByLocalpart(ctx, nil, localpart, "")
}
