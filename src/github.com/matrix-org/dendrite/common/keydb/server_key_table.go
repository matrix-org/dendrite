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

package keydb

import (
	"database/sql"
	"encoding/json"

	"github.com/lib/pq"
	"github.com/matrix-org/gomatrixserverlib"
)

const serverKeysSchema = `
-- A cache of server keys downloaded from remote servers.
CREATE TABLE IF NOT EXISTS keydb_server_keys (
	-- The name of the matrix server the key is for.
	server_name TEXT NOT NULL,
	-- The ID of the server key.
	server_key_id TEXT NOT NULL,
	-- Combined server name and key ID separated by the ASCII unit separator
	-- to make it easier to run bulk queries.
	server_name_and_key_id TEXT NOT NULL,
	-- When the keys are valid until as a millisecond timestamp.
	valid_until_ts BIGINT NOT NULL,
	-- The raw JSON for the server key.
	server_key_json TEXT NOT NULL,
	CONSTRAINT keydb_server_keys_unique UNIQUE (server_name, server_key_id)
);

CREATE INDEX IF NOT EXISTS keydb_server_name_and_key_id ON keydb_server_keys (server_name_and_key_id);
`

const bulkSelectServerKeysSQL = "" +
	"SELECT server_name, server_key_id, server_key_json FROM keydb_server_keys" +
	" WHERE server_name_and_key_id = ANY($1)"

const upsertServerKeysSQL = "" +
	"INSERT INTO keydb_server_keys (server_name, server_key_id," +
	" server_name_and_key_id, valid_until_ts, server_key_json)" +
	" VALUES ($1, $2, $3, $4, $5)" +
	" ON CONFLICT ON CONSTRAINT keydb_server_keys_unique" +
	" DO UPDATE SET valid_until_ts = $4, server_key_json = $5"

type serverKeyStatements struct {
	bulkSelectServerKeysStmt *sql.Stmt
	upsertServerKeysStmt     *sql.Stmt
}

func (s *serverKeyStatements) prepare(db *sql.DB) (err error) {
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
	requests map[gomatrixserverlib.PublicKeyRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyRequest]gomatrixserverlib.ServerKeys, error) {
	var nameAndKeyIDs []string
	for request := range requests {
		nameAndKeyIDs = append(nameAndKeyIDs, nameAndKeyID(request))
	}
	rows, err := s.bulkSelectServerKeysStmt.Query(pq.StringArray(nameAndKeyIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	results := map[gomatrixserverlib.PublicKeyRequest]gomatrixserverlib.ServerKeys{}
	for rows.Next() {
		var serverName string
		var keyID string
		var keyJSON []byte
		if err := rows.Scan(&serverName, &keyID, &keyJSON); err != nil {
			return nil, err
		}
		var serverKeys gomatrixserverlib.ServerKeys
		if err := json.Unmarshal(keyJSON, &serverKeys); err != nil {
			return nil, err
		}
		r := gomatrixserverlib.PublicKeyRequest{
			ServerName: gomatrixserverlib.ServerName(serverName),
			KeyID:      gomatrixserverlib.KeyID(keyID),
		}
		results[r] = serverKeys
	}
	return results, nil
}

func (s *serverKeyStatements) upsertServerKeys(
	request gomatrixserverlib.PublicKeyRequest, keys gomatrixserverlib.ServerKeys,
) error {
	keyJSON, err := json.Marshal(keys)
	if err != nil {
		return err
	}
	_, err = s.upsertServerKeysStmt.Exec(
		string(request.ServerName), string(request.KeyID), nameAndKeyID(request),
		int64(keys.ValidUntilTS), keyJSON,
	)
	if err != nil {
		return err
	}
	return nil
}

func nameAndKeyID(request gomatrixserverlib.PublicKeyRequest) string {
	return string(request.ServerName) + "\x1F" + string(request.KeyID)
}
