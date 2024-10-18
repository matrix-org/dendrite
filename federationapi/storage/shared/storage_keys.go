// Copyright 2024 New Vector Ltd.
// Copyright 2019, 2020 The Matrix.org Foundation C.I.C.
// Copyright 2017, 2018 New Vector Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package shared

import (
	"context"
	"database/sql"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

// FetcherName implements KeyFetcher
func (d Database) FetcherName() string {
	return "FederationAPIKeyDatabase"
}

// FetchKeys implements gomatrixserverlib.KeyDatabase
func (d *Database) FetchKeys(
	ctx context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]spec.Timestamp,
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
