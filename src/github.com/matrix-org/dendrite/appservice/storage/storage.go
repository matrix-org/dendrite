// Copyright 2018 New Vector Ltd
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

package storage

import (
	"context"
	"database/sql"

	// Import postgres database driver
	_ "github.com/lib/pq"
	"github.com/matrix-org/gomatrixserverlib"
)

// Database stores events intended to be later sent to application services
type Database struct {
	events eventsStatements
	txnID  txnStatements
	db     *sql.DB
}

// NewDatabase opens a new database
func NewDatabase(dataSourceName string) (*Database, error) {
	var result Database
	var err error
	if result.db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
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

// StoreEvent takes in a gomatrixserverlib.Event and stores it in the database
// for a transaction worker to pull and later send to an application service.
func (d *Database) StoreEvent(
	ctx context.Context,
	appServiceID string,
	event gomatrixserverlib.Event,
) error {
	return d.events.insertEvent(ctx, appServiceID, event)
}

// GetEventsWithAppServiceID returns a slice of events and their IDs intended to
// be sent to an application service given its ID.
func (d *Database) GetEventsWithAppServiceID(
	ctx context.Context,
	appServiceID string,
	limit int,
) ([]string, []gomatrixserverlib.ApplicationServiceEvent, error) {
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

// RemoveEventsByID removes events from the database given a slice of their
// event IDs.
func (d *Database) RemoveEventsByID(
	ctx context.Context,
	eventIDs []string,
) error {
	return d.events.deleteEventsByID(ctx, eventIDs)
}

// GetTxnIDWithAppServiceID takes in an application service ID and returns the
// last used transaction ID associated with it.
func (d *Database) GetTxnIDWithAppServiceID(
	ctx context.Context,
	appServiceID string,
) (int, error) {
	return d.txnID.selectTxnID(ctx, appServiceID)
}

// UpsertTxnIDWithAppServiceID takes in an application service ID and a
// transaction ID and stores them in the DB, unless the pair already exists, in
// which case it updates them.
func (d *Database) UpsertTxnIDWithAppServiceID(
	ctx context.Context,
	appServiceID string,
	txnID int,
) error {
	return d.txnID.upsertTxnID(ctx, appServiceID, txnID)
}
