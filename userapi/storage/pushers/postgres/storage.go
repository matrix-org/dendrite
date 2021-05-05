// Copyright 2021 Dan Peleg <dan@globekeeper.com>
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

package postgres

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// Database represents a pusher database.
type Database struct {
	db      *sql.DB
	pushers pushersStatements
}

// NewDatabase creates a new puser database
func NewDatabase(dbProperties *config.DatabaseOptions, serverName gomatrixserverlib.ServerName) (*Database, error) {
	db, err := sqlutil.Open(dbProperties)
	if err != nil {
		return nil, err
	}
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

	if err = d.prepare(db, serverName); err != nil {
		return nil, err
	}

	return &Database{db, d}, nil
}

func (d *Database) CreatePusher(
	ctx context.Context, session_id int64,
	pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string,
) error {
	return d.pushers.insertPusher(ctx, nil, session_id, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart)
}

// GetPushersByLocalpart returns the pushers matching the given localpart.
func (d *Database) GetPushersByLocalpart(
	ctx context.Context, localpart string,
) ([]api.Pusher, error) {
	return d.pushers.selectPushersByLocalpart(ctx, nil, localpart)
}

// GetPusherByPushkey returns the pusher matching the given localpart.
func (d *Database) GetPusherByPushkey(
	ctx context.Context, pushkey, localpart string,
) (*api.Pusher, error) {
	return d.pushers.selectPusherByPushkey(ctx, localpart, pushkey)
}

// UpdatePusher updates the given pusher with the display name.
// Returns SQL error if there are problems and nil on success.
func (d *Database) UpdatePusher(
	ctx context.Context, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string,
) error {
	return sqlutil.WithTransaction(d.db, func(txn *sql.Tx) error {
		return d.pushers.updatePusher(ctx, txn, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart)
	})
}

// RemovePusher revokes a pusher by deleting the entry in the database
// matching with the given pushkey and user ID localpart.
// If the pusher doesn't exist, it will not return an error
// If something went wrong during the deletion, it will return the SQL error.
func (d *Database) RemovePusher(
	ctx context.Context, appid, pushkey, localpart string,
) error {
	return sqlutil.WithTransaction(d.db, func(txn *sql.Tx) error {
		if err := d.pushers.deletePusher(ctx, txn, appid, pushkey, localpart); err != sql.ErrNoRows {
			return err
		} else {
			logrus.WithError(err).Debug("RemovePusher Error")
		}
		return nil
	})
}
