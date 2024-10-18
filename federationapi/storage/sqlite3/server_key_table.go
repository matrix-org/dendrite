// Copyright 2024 New Vector Ltd.
// Copyright 2019, 2020 The Matrix.org Foundation C.I.C.
// Copyright 2017, 2018 New Vector Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package sqlite3

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

const serverSigningKeysSchema = `
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

const bulkSelectServerSigningKeysSQL = "" +
	"SELECT server_name, server_key_id, valid_until_ts, expired_ts, " +
	"   server_key FROM keydb_server_keys" +
	" WHERE server_name_and_key_id IN ($1)"

const upsertServerSigningKeysSQL = "" +
	"INSERT INTO keydb_server_keys (server_name, server_key_id," +
	" server_name_and_key_id, valid_until_ts, expired_ts, server_key)" +
	" VALUES ($1, $2, $3, $4, $5, $6)" +
	" ON CONFLICT (server_name, server_key_id)" +
	" DO UPDATE SET valid_until_ts = $4, expired_ts = $5, server_key = $6"

type serverSigningKeyStatements struct {
	db                       *sql.DB
	bulkSelectServerKeysStmt *sql.Stmt
	upsertServerKeysStmt     *sql.Stmt
}

func NewSQLiteServerSigningKeysTable(db *sql.DB) (s *serverSigningKeyStatements, err error) {
	s = &serverSigningKeyStatements{
		db: db,
	}
	_, err = db.Exec(serverSigningKeysSchema)
	if err != nil {
		return
	}
	return s, sqlutil.StatementList{
		{&s.bulkSelectServerKeysStmt, bulkSelectServerSigningKeysSQL},
		{&s.upsertServerKeysStmt, upsertServerSigningKeysSQL},
	}.Prepare(db)
}

func (s *serverSigningKeyStatements) BulkSelectServerKeys(
	ctx context.Context, txn *sql.Tx,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]spec.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	nameAndKeyIDs := make([]string, 0, len(requests))
	for request := range requests {
		nameAndKeyIDs = append(nameAndKeyIDs, nameAndKeyID(request))
	}
	results := make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, len(requests))
	iKeyIDs := make([]interface{}, len(nameAndKeyIDs))
	for i, v := range nameAndKeyIDs {
		iKeyIDs[i] = v
	}

	err := sqlutil.RunLimitedVariablesQuery(
		ctx, bulkSelectServerSigningKeysSQL, s.db, iKeyIDs, sqlutil.SQLite3MaxVariables,
		func(rows *sql.Rows) error {
			var serverName string
			var keyID string
			var key string
			var validUntilTS int64
			var expiredTS int64
			var vk gomatrixserverlib.VerifyKey
			for rows.Next() {
				if err := rows.Scan(&serverName, &keyID, &validUntilTS, &expiredTS, &key); err != nil {
					return fmt.Errorf("bulkSelectServerKeys: %v", err)
				}
				r := gomatrixserverlib.PublicKeyLookupRequest{
					ServerName: spec.ServerName(serverName),
					KeyID:      gomatrixserverlib.KeyID(keyID),
				}
				err := vk.Key.Decode(key)
				if err != nil {
					return fmt.Errorf("bulkSelectServerKeys: %v", err)
				}
				results[r] = gomatrixserverlib.PublicKeyLookupResult{
					VerifyKey:    vk,
					ValidUntilTS: spec.Timestamp(validUntilTS),
					ExpiredTS:    spec.Timestamp(expiredTS),
				}
			}
			return nil
		},
	)

	if err != nil {
		return nil, err
	}
	return results, nil
}

func (s *serverSigningKeyStatements) UpsertServerKeys(
	ctx context.Context, txn *sql.Tx,
	request gomatrixserverlib.PublicKeyLookupRequest,
	key gomatrixserverlib.PublicKeyLookupResult,
) error {
	stmt := sqlutil.TxStmt(txn, s.upsertServerKeysStmt)
	_, err := stmt.ExecContext(
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
