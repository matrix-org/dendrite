// Copyright 2022 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
)

const ssoSchema = `
-- Stores data about SSO associations.
CREATE TABLE IF NOT EXISTS account_sso (
	-- The "iss" namespace. Must be "oidc".
	namespace TEXT NOT NULL,
    -- The issuer; for "oidc", a URL.
	iss TEXT NOT NULL,
    -- The subject (user ID).
	sub TEXT NOT NULL,
	-- The localpart of the Matrix user ID associated to this 3PID
	localpart TEXT NOT NULL,

	PRIMARY KEY(namespace, iss, sub)
);
`

const selectLocalpartForSSOSQL = "" +
	"SELECT localpart FROM account_sso WHERE namespace = $1 AND iss = $2 AND sub = $3"

const insertSSOSQL = "" +
	"INSERT INTO account_sso (namespace, iss, sub, localpart) VALUES ($1, $2, $3, $4)"

const deleteSSOSQL = "" +
	"DELETE FROM account_sso WHERE namespace = $1 AND iss = $2 AND sub = $3"

type ssoStatements struct {
	selectLocalpartForSSOStmt *sql.Stmt
	insertSSOStmt             *sql.Stmt
	deleteSSOStmt             *sql.Stmt
}

func NewPostgresSSOTable(db *sql.DB) (tables.SSOTable, error) {
	s := &ssoStatements{}
	_, err := db.Exec(ssoSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.selectLocalpartForSSOStmt, selectLocalpartForSSOSQL},
		{&s.insertSSOStmt, insertSSOSQL},
		{&s.deleteSSOStmt, deleteSSOSQL},
	}.Prepare(db)
}

func (s *ssoStatements) SelectLocalpartForSSO(
	ctx context.Context, txn *sql.Tx, namespace, iss, sub string,
) (localpart string, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectLocalpartForSSOStmt)
	err = stmt.QueryRowContext(ctx, namespace, iss, sub).Scan(&localpart)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return
}

func (s *ssoStatements) InsertSSO(
	ctx context.Context, txn *sql.Tx, namespace, iss, sub, localpart string,
) (err error) {
	stmt := sqlutil.TxStmt(txn, s.insertSSOStmt)
	_, err = stmt.ExecContext(ctx, namespace, iss, sub, localpart)
	return
}

func (s *ssoStatements) DeleteSSO(
	ctx context.Context, txn *sql.Tx, namespace, iss, sub string) (err error) {
	stmt := sqlutil.TxStmt(txn, s.deleteSSOStmt)
	_, err = stmt.ExecContext(ctx, namespace, iss, sub)
	return
}
