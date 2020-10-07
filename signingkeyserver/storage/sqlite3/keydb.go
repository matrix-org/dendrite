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

	"golang.org/x/crypto/ed25519"

	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"

	_ "github.com/mattn/go-sqlite3"
)

// A Database implements gomatrixserverlib.KeyDatabase and is used to store
// the public keys for other matrix servers.
type Database struct {
	writer     sqlutil.Writer
	statements serverKeyStatements
}

// NewDatabase prepares a new key database.
// It creates the necessary tables if they don't already exist.
// It prepares all the SQL statements that it will use.
// Returns an error if there was a problem talking to the database.
func NewDatabase(
	dbProperties *config.DatabaseOptions,
	serverName gomatrixserverlib.ServerName,
	serverKey ed25519.PublicKey,
	serverKeyID gomatrixserverlib.KeyID,
) (*Database, error) {
	db, err := sqlutil.Open(dbProperties)
	if err != nil {
		return nil, err
	}
	d := &Database{
		writer: sqlutil.NewExclusiveWriter(),
	}
	err = d.statements.prepare(db, d.writer)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return d, nil
}

// FetcherName implements KeyFetcher
func (d Database) FetcherName() string {
	return "SqliteKeyDatabase"
}

// FetchKeys implements gomatrixserverlib.KeyDatabase
func (d *Database) FetchKeys(
	ctx context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	return d.statements.bulkSelectServerKeys(ctx, requests)
}

// StoreKeys implements gomatrixserverlib.KeyDatabase
func (d *Database) StoreKeys(
	ctx context.Context,
	keyMap map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult,
) error {
	// TODO: Inserting all the keys within a single transaction may
	// be more efficient since the transaction overhead can be quite
	// high for a single insert statement.
	var lastErr error
	for request, keys := range keyMap {
		if err := d.statements.upsertServerKeys(ctx, request, keys); err != nil {
			// Rather than returning immediately on error we try to insert the
			// remaining keys.
			// Since we are inserting the keys outside of a transaction it is
			// possible for some of the inserts to succeed even though some
			// of the inserts have failed.
			// Ensuring that we always insert all the keys we can means that
			// this behaviour won't depend on the iteration order of the map.
			lastErr = err
		}
	}
	return lastErr
}
