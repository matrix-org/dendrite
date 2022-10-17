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
	"time"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
	"github.com/matrix-org/util"
)

const loginTokenSchema = `
CREATE TABLE IF NOT EXISTS userapi_login_tokens (
	-- The random value of the token issued to a user
	token TEXT NOT NULL PRIMARY KEY,
	-- When the token expires
	token_expires_at TIMESTAMP NOT NULL,

    -- The mxid for this account
	user_id TEXT NOT NULL
);

-- This index allows efficient garbage collection of expired tokens.
CREATE INDEX IF NOT EXISTS userapi_login_tokens_expiration_idx ON userapi_login_tokens(token_expires_at);
`

const insertLoginTokenSQL = "" +
	"INSERT INTO userapi_login_tokens(token, token_expires_at, user_id) VALUES ($1, $2, $3)"

const deleteLoginTokenSQL = "" +
	"DELETE FROM userapi_login_tokens WHERE token = $1 OR token_expires_at <= $2"

const selectLoginTokenSQL = "" +
	"SELECT user_id FROM userapi_login_tokens WHERE token = $1 AND token_expires_at > $2"

type loginTokenStatements struct {
	insertStmt *sql.Stmt
	deleteStmt *sql.Stmt
	selectStmt *sql.Stmt
}

func NewPostgresLoginTokenTable(db *sql.DB) (tables.LoginTokenTable, error) {
	s := &loginTokenStatements{}
	_, err := db.Exec(loginTokenSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.insertStmt, insertLoginTokenSQL},
		{&s.deleteStmt, deleteLoginTokenSQL},
		{&s.selectStmt, selectLoginTokenSQL},
	}.Prepare(db)
}

// insert adds an already generated token to the database.
func (s *loginTokenStatements) InsertLoginToken(ctx context.Context, txn *sql.Tx, metadata *api.LoginTokenMetadata, data *api.LoginTokenData) error {
	stmt := sqlutil.TxStmt(txn, s.insertStmt)
	_, err := stmt.ExecContext(ctx, metadata.Token, metadata.Expiration.UTC(), data.UserID)
	return err
}

// deleteByToken removes the named token.
//
// As a simple way to garbage-collect stale tokens, we also remove all expired tokens.
// The userapi_login_tokens_expiration_idx index should make that efficient.
func (s *loginTokenStatements) DeleteLoginToken(ctx context.Context, txn *sql.Tx, token string) error {
	stmt := sqlutil.TxStmt(txn, s.deleteStmt)
	res, err := stmt.ExecContext(ctx, token, time.Now().UTC())
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n > 1 {
		util.GetLogger(ctx).WithField("num_deleted", n).Infof("Deleted %d login tokens (%d likely additional expired token)", n, n-1)
	}
	return nil
}

// selectByToken returns the data associated with the given token. May return sql.ErrNoRows.
func (s *loginTokenStatements) SelectLoginToken(ctx context.Context, token string) (*api.LoginTokenData, error) {
	var data api.LoginTokenData
	err := s.selectStmt.QueryRowContext(ctx, token, time.Now().UTC()).Scan(&data.UserID)
	if err != nil {
		return nil, err
	}

	return &data, nil
}
