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
	"database/sql"
	"strings"

	lru "github.com/hashicorp/golang-lru"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const serverKeysSchema = `
-- A cache of signing keys downloaded from remote servers.
CREATE TABLE IF NOT EXISTS keydb_server_keys (
	-- The name of the matrix server the key is for.
	server_name TEXT NOT NULL,
	-- The ID of the server key.
	server_key_id TEXT NOT NULL,
	-- Combined server name and key ID separated by the ASCII unit separator
	-- to make it easier to run bulk queries.
	server_name_and_key_id TEXT NOT NULL,
	-- When the key is valid until as a millisecond timestamp.
	-- 0 if this is an expired key (in which case expired_ts will be non-zero)
	valid_until_ts BIGINT NOT NULL,
	-- When the key expired as a millisecond timestamp.
	-- 0 if this is an active key (in which case valid_until_ts will be non-zero)
	expired_ts BIGINT NOT NULL,
	-- The base64-encoded public key.
	server_key TEXT NOT NULL,
	UNIQUE (server_name, server_key_id)
);

CREATE INDEX IF NOT EXISTS keydb_server_name_and_key_id ON keydb_server_keys (server_name_and_key_id);
`

const bulkSelectServerKeysSQL = "" +
	"SELECT server_name, server_key_id, valid_until_ts, expired_ts, " +
	"   server_key FROM keydb_server_keys" +
	" WHERE server_name_and_key_id IN ($1)"

const upsertServerKeysSQL = "" +
	"INSERT INTO keydb_server_keys (server_name, server_key_id," +
	" server_name_and_key_id, valid_until_ts, expired_ts, server_key)" +
	" VALUES ($1, $2, $3, $4, $5, $6)" +
	" ON CONFLICT (server_name, server_key_id)" +
	" DO UPDATE SET valid_until_ts = $4, expired_ts = $5, server_key = $6"

type serverKeyStatements struct {
	db                       *sql.DB
	bulkSelectServerKeysStmt *sql.Stmt
	upsertServerKeysStmt     *sql.Stmt

	cache *lru.Cache // nameAndKeyID => gomatrixserverlib.PublicKeyLookupResult
}

func (s *serverKeyStatements) prepare(db *sql.DB) (err error) {
	s.db = db
	s.cache, err = lru.New(64)
	if err != nil {
		return
	}
	_, err = db.Exec(serverKeysSchema)
	if err != nil {
		return
	}
	if s.bulkSelectServerKeysStmt, err = db.Prepare(bulkSelectServerKeysSQL); err != nil {
		return
	}
	if s.upsertServerKeysStmt, err = db.Prepare(upsertServerKeysSQL); err != nil {
		return
	}
	return
}

func (s *serverKeyStatements) bulkSelectServerKeys(
	ctx context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	var nameAndKeyIDs []string
	for request := range requests {
		nameAndKeyIDs = append(nameAndKeyIDs, nameAndKeyID(request))
	}

	// If we can satisfy all of the requests from the cache, do so. TODO: Allow partial matches with merges.
	cacheResults := map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult{}
	for request := range requests {
		r, ok := s.cache.Get(nameAndKeyID(request))
		if !ok {
			break
		}
		cacheResult := r.(gomatrixserverlib.PublicKeyLookupResult)
		cacheResults[request] = cacheResult
	}
	if len(cacheResults) == len(requests) {
		util.GetLogger(ctx).Infof("KeyDB cache hit for %d keys", len(cacheResults))
		return cacheResults, nil
	}

	query := strings.Replace(bulkSelectServerKeysSQL, "($1)", common.QueryVariadic(len(nameAndKeyIDs)), 1)

	iKeyIDs := make([]interface{}, len(nameAndKeyIDs))
	for i, v := range nameAndKeyIDs {
		iKeyIDs[i] = v
	}

	rows, err := s.db.QueryContext(ctx, query, iKeyIDs...)
	if err != nil {
		return nil, err
	}
	defer common.LogIfError(ctx, rows.Close(), "bulkSelectServerKeys: rows.close() failed")
	results := map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult{}
	for rows.Next() {
		var serverName string
		var keyID string
		var key string
		var validUntilTS int64
		var expiredTS int64
		if err = rows.Scan(&serverName, &keyID, &validUntilTS, &expiredTS, &key); err != nil {
			return nil, err
		}
		r := gomatrixserverlib.PublicKeyLookupRequest{
			ServerName: gomatrixserverlib.ServerName(serverName),
			KeyID:      gomatrixserverlib.KeyID(keyID),
		}
		vk := gomatrixserverlib.VerifyKey{}
		err = vk.Key.Decode(key)
		if err != nil {
			return nil, err
		}
		results[r] = gomatrixserverlib.PublicKeyLookupResult{
			VerifyKey:    vk,
			ValidUntilTS: gomatrixserverlib.Timestamp(validUntilTS),
			ExpiredTS:    gomatrixserverlib.Timestamp(expiredTS),
		}
	}
	return results, nil
}

func (s *serverKeyStatements) upsertServerKeys(
	ctx context.Context,
	request gomatrixserverlib.PublicKeyLookupRequest,
	key gomatrixserverlib.PublicKeyLookupResult,
) error {
	s.cache.Add(nameAndKeyID(request), key)
	_, err := s.upsertServerKeysStmt.ExecContext(
		ctx,
		string(request.ServerName),
		string(request.KeyID),
		nameAndKeyID(request),
		key.ValidUntilTS,
		key.ExpiredTS,
		key.Key.Encode(),
	)
	return err
}

func nameAndKeyID(request gomatrixserverlib.PublicKeyLookupRequest) string {
	return string(request.ServerName) + "\x1F" + string(request.KeyID)
}
