// Copyright 2021 The Matrix.org Foundation C.I.C.
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
	"encoding/json"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/federationapi/storage/tables"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/gomatrixserverlib"
)

const notaryServerKeysMetadataSchema = `
CREATE TABLE IF NOT EXISTS federationsender_notary_server_keys_metadata (
    notary_id BIGINT NOT NULL,
	server_name TEXT NOT NULL,
	key_id TEXT NOT NULL,
	UNIQUE (server_name, key_id)
);
`

const upsertServerKeysSQL = "" +
	"INSERT INTO federationsender_notary_server_keys_metadata (notary_id, server_name, key_id) VALUES ($1, $2, $3)" +
	" ON CONFLICT (server_name, key_id) DO UPDATE SET notary_id = $1"

// for a given (server_name, key_id), find the existing notary ID and valid until. Used to check if we will replace it
// JOINs with the json table
const selectNotaryKeyMetadataSQL = `
	SELECT federationsender_notary_server_keys_metadata.notary_id, valid_until FROM federationsender_notary_server_keys_json
	JOIN federationsender_notary_server_keys_metadata ON
	federationsender_notary_server_keys_metadata.notary_id = federationsender_notary_server_keys_json.notary_id
	WHERE federationsender_notary_server_keys_metadata.server_name = $1 AND federationsender_notary_server_keys_metadata.key_id = $2
`

// select the response which has the highest valid_until value
// JOINs with the json table
const selectNotaryKeyResponsesSQL = `
	SELECT response_json FROM federationsender_notary_server_keys_json
	WHERE server_name = $1 AND valid_until = (
		SELECT MAX(valid_until) FROM federationsender_notary_server_keys_json WHERE server_name = $1
	)
`

// select the responses which have the given key IDs
// JOINs with the json table
const selectNotaryKeyResponsesWithKeyIDsSQL = `
	SELECT response_json FROM federationsender_notary_server_keys_json
	JOIN federationsender_notary_server_keys_metadata ON
	federationsender_notary_server_keys_metadata.notary_id = federationsender_notary_server_keys_json.notary_id
	WHERE federationsender_notary_server_keys_json.server_name = $1 AND federationsender_notary_server_keys_metadata.key_id = ANY ($2)
	GROUP BY federationsender_notary_server_keys_json.notary_id
`

// JOINs with the metadata table
const deleteUnusedServerKeysJSONSQL = `
	DELETE FROM federationsender_notary_server_keys_json WHERE federationsender_notary_server_keys_json.notary_id NOT IN (
		SELECT DISTINCT notary_id FROM federationsender_notary_server_keys_metadata
	)
`

type notaryServerKeysMetadataStatements struct {
	db                                     *sql.DB
	upsertServerKeysStmt                   *sql.Stmt
	selectNotaryKeyResponsesStmt           *sql.Stmt
	selectNotaryKeyResponsesWithKeyIDsStmt *sql.Stmt
	selectNotaryKeyMetadataStmt            *sql.Stmt
	deleteUnusedServerKeysJSONStmt         *sql.Stmt
}

func NewPostgresNotaryServerKeysMetadataTable(db *sql.DB) (s *notaryServerKeysMetadataStatements, err error) {
	s = &notaryServerKeysMetadataStatements{
		db: db,
	}
	_, err = db.Exec(notaryServerKeysMetadataSchema)
	if err != nil {
		return
	}

	if s.upsertServerKeysStmt, err = db.Prepare(upsertServerKeysSQL); err != nil {
		return
	}
	if s.selectNotaryKeyResponsesStmt, err = db.Prepare(selectNotaryKeyResponsesSQL); err != nil {
		return
	}
	if s.selectNotaryKeyResponsesWithKeyIDsStmt, err = db.Prepare(selectNotaryKeyResponsesWithKeyIDsSQL); err != nil {
		return
	}
	if s.selectNotaryKeyMetadataStmt, err = db.Prepare(selectNotaryKeyMetadataSQL); err != nil {
		return
	}
	if s.deleteUnusedServerKeysJSONStmt, err = db.Prepare(deleteUnusedServerKeysJSONSQL); err != nil {
		return
	}
	return
}

func (s *notaryServerKeysMetadataStatements) UpsertKey(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, keyID gomatrixserverlib.KeyID, newNotaryID tables.NotaryID, newValidUntil gomatrixserverlib.Timestamp,
) (tables.NotaryID, error) {
	notaryID := newNotaryID
	// see if the existing notary ID a) exists, b) has a longer valid_until
	var existingNotaryID tables.NotaryID
	var existingValidUntil gomatrixserverlib.Timestamp
	if err := txn.Stmt(s.selectNotaryKeyMetadataStmt).QueryRowContext(ctx, serverName, keyID).Scan(&existingNotaryID, &existingValidUntil); err != nil {
		if err != sql.ErrNoRows {
			return 0, err
		}
	}
	if existingValidUntil.Time().After(newValidUntil.Time()) {
		// the existing valid_until is valid longer, so use that.
		return existingNotaryID, nil
	}
	// overwrite the notary_id for this (server_name, key_id) tuple
	_, err := txn.Stmt(s.upsertServerKeysStmt).ExecContext(ctx, notaryID, serverName, keyID)
	return notaryID, err
}

func (s *notaryServerKeysMetadataStatements) SelectKeys(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, keyIDs []gomatrixserverlib.KeyID) ([]gomatrixserverlib.ServerKeys, error) {
	var rows *sql.Rows
	var err error
	if len(keyIDs) == 0 {
		rows, err = txn.Stmt(s.selectNotaryKeyResponsesStmt).QueryContext(ctx, string(serverName))
	} else {
		keyIDstr := make([]string, len(keyIDs))
		for i := range keyIDs {
			keyIDstr[i] = string(keyIDs[i])
		}
		rows, err = txn.Stmt(s.selectNotaryKeyResponsesWithKeyIDsStmt).QueryContext(ctx, string(serverName), pq.StringArray(keyIDstr))
	}
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectNotaryKeyResponsesStmt close failed")
	var results []gomatrixserverlib.ServerKeys
	for rows.Next() {
		var sk gomatrixserverlib.ServerKeys
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &sk); err != nil {
			return nil, err
		}
		results = append(results, sk)
	}
	return results, nil
}

func (s *notaryServerKeysMetadataStatements) DeleteOldJSONResponses(ctx context.Context, txn *sql.Tx) error {
	_, err := txn.Stmt(s.deleteUnusedServerKeysJSONStmt).ExecContext(ctx)
	return err
}
