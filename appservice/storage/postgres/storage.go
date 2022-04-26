// Copyright 2018 New Vector Ltd
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

package postgres

import (
	"context"
	"database/sql"

	// Import postgres database driver
	_ "github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

// Database stores events intended to be later sent to application services
type Database struct {
	events eventsStatements
	txnID  txnStatements
	db     *sql.DB
	writer sqlutil.Writer
}

// NewDatabase opens a new database
func NewDatabase(dbProperties *config.DatabaseOptions) (*Database, error) {
	var result Database
	var err error
	if result.db, err = sqlutil.Open(dbProperties); err != nil {
		return nil, err
	}
	result.writer = sqlutil.NewDummyWriter()
	if err = result.prepare(); err != nil {
		return nil, err
	}
	return &result, nil
}

func (d *Database) prepare() error {
	if err := d.events.prepare(d.db); err != nil {
		return err
	}

	return d.txnID.prepare(d.db)
}

// StoreEvent takes in a gomatrixserverlib.HeaderedEvent and stores it in the database
// for a transaction worker to pull and later send to an application service.
func (d *Database) StoreEvent(
	ctx context.Context,
	appServiceID string,
	event *gomatrixserverlib.HeaderedEvent,
) error {
	return d.events.insertEvent(ctx, appServiceID, event)
}

// GetEventsWithAppServiceID returns a slice of events and their IDs intended to
// be sent to an application service given its ID.
func (d *Database) GetEventsWithAppServiceID(
	ctx context.Context,
	appServiceID string,
	limit int,
) (int, int, []gomatrixserverlib.HeaderedEvent, bool, error) {
	return d.events.selectEventsByApplicationServiceID(ctx, appServiceID, limit)
}

// CountEventsWithAppServiceID returns the number of events destined for an
// application service given its ID.
func (d *Database) CountEventsWithAppServiceID(
	ctx context.Context,
	appServiceID string,
) (int, error) {
	return d.events.countEventsByApplicationServiceID(ctx, appServiceID)
}

// UpdateTxnIDForEvents takes in an application service ID and a
// and stores them in the DB, unless the pair already exists, in
// which case it updates them.
func (d *Database) UpdateTxnIDForEvents(
	ctx context.Context,
	appserviceID string,
	maxID, txnID int,
) error {
	return d.events.updateTxnIDForEvents(ctx, appserviceID, maxID, txnID)
}

// RemoveEventsBeforeAndIncludingID removes all events from the database that
// are less than or equal to a given maximum ID. IDs here are implemented as a
// serial, thus this should always delete events in chronological order.
func (d *Database) RemoveEventsBeforeAndIncludingID(
	ctx context.Context,
	appserviceID string,
	eventTableID int,
) error {
	return d.events.deleteEventsBeforeAndIncludingID(ctx, appserviceID, eventTableID)
}

// GetLatestTxnID returns the latest available transaction id
func (d *Database) GetLatestTxnID(
	ctx context.Context,
) (int, error) {
	return d.txnID.selectTxnID(ctx)
}
