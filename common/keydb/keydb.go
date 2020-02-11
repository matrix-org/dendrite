// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package keydb

import (
	"context"
	"net/url"

	"golang.org/x/crypto/ed25519"

	"github.com/matrix-org/dendrite/common/keydb/postgres"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	FetcherName() string
	FetchKeys(ctx context.Context, requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error)
	StoreKeys(ctx context.Context, keyMap map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult) error
}

// NewDatabase opens a database connection.
func NewDatabase(
	dataSourceName string,
	serverName gomatrixserverlib.ServerName,
	serverKey ed25519.PublicKey,
	serverKeyID gomatrixserverlib.KeyID,
) (Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return postgres.NewDatabase(dataSourceName, serverName, serverKey, serverKeyID)
	}
	switch uri.Scheme {
	case "postgres":
		return postgres.NewDatabase(dataSourceName, serverName, serverKey, serverKeyID)
	default:
		return postgres.NewDatabase(dataSourceName, serverName, serverKey, serverKeyID)
	}
}
