// Copyright 2024 New Vector Ltd.
// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"

	"github.com/element-hq/dendrite/federationapi/storage/tables"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

const notaryServerKeysJSONSchema = `
CREATE SEQUENCE IF NOT EXISTS federationsender_notary_server_keys_json_pkey;
CREATE TABLE IF NOT EXISTS federationsender_notary_server_keys_json (
    notary_id BIGINT PRIMARY KEY NOT NULL DEFAULT nextval('federationsender_notary_server_keys_json_pkey'),
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

func NewPostgresNotaryServerKeysTable(db *sql.DB) (s *notaryServerKeysStatements, err error) {
	s = &notaryServerKeysStatements{
		db: db,
	}
	_, err = db.Exec(notaryServerKeysJSONSchema)
	if err != nil {
		return
	}

	return s, sqlutil.StatementList{
		{&s.insertServerKeysJSONStmt, insertServerKeysJSONSQL},
	}.Prepare(db)
}

func (s *notaryServerKeysStatements) InsertJSONResponse(
	ctx context.Context, txn *sql.Tx, keyQueryResponseJSON gomatrixserverlib.ServerKeys, serverName spec.ServerName, validUntil spec.Timestamp,
) (tables.NotaryID, error) {
	var notaryID tables.NotaryID
	return notaryID, txn.Stmt(s.insertServerKeysJSONStmt).QueryRowContext(ctx, string(keyQueryResponseJSON.Raw), serverName, validUntil).Scan(&notaryID)
}
