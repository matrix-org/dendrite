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

package sqlite3

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/federationapi/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"
)

const notaryServerKeysJSONSchema = `
CREATE TABLE IF NOT EXISTS federationsender_notary_server_keys_json (
    notary_id INTEGER PRIMARY KEY AUTOINCREMENT,
	response_json TEXT NOT NULL,
	server_name TEXT NOT NULL,
	valid_until BIGINT NOT NULL
);
`

const insertServerKeysJSONSQL = "" +
	"INSERT INTO federationsender_notary_server_keys_json (response_json, server_name, valid_until) VALUES ($1, $2, $3)" +
	" RETURNING notary_id"

type notaryServerKeysStatements struct {
	db                       *sql.DB
	insertServerKeysJSONStmt *sql.Stmt
}

func NewSQLiteNotaryServerKeysTable(db *sql.DB) (s *notaryServerKeysStatements, err error) {
	s = &notaryServerKeysStatements{
		db: db,
	}
	_, err = db.Exec(notaryServerKeysJSONSchema)
	if err != nil {
		return
	}

	if s.insertServerKeysJSONStmt, err = db.Prepare(insertServerKeysJSONSQL); err != nil {
		return
	}
	return
}

func (s *notaryServerKeysStatements) InsertJSONResponse(
	ctx context.Context, txn *sql.Tx, keyQueryResponseJSON gomatrixserverlib.ServerKeys, serverName gomatrixserverlib.ServerName, validUntil gomatrixserverlib.Timestamp,
) (tables.NotaryID, error) {
	var notaryID tables.NotaryID
	return notaryID, txn.Stmt(s.insertServerKeysJSONStmt).QueryRowContext(ctx, string(keyQueryResponseJSON.Raw), serverName, validUntil).Scan(&notaryID)
}
