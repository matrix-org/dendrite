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

package shared

import (
	"context"
	"database/sql"

	"github.com/matrix-org/gomatrixserverlib"
)

// FetcherName implements KeyFetcher
func (d Database) FetcherName() string {
	return "FederationAPIKeyDatabase"
}

// FetchKeys implements gomatrixserverlib.KeyDatabase
func (d *Database) FetchKeys(
	ctx context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	return d.ServerSigningKeys.BulkSelectServerKeys(ctx, nil, requests)
}

// StoreKeys implements gomatrixserverlib.KeyDatabase
func (d *Database) StoreKeys(
	ctx context.Context,
	keyMap map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult,
) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		var lastErr error
		for request, keys := range keyMap {
			if err := d.ServerSigningKeys.UpsertServerKeys(ctx, txn, request, keys); err != nil {
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
	})
}
