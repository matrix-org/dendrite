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

package sqlite3

import (
	"context"
	"database/sql"
	"time"

	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"

	log "github.com/sirupsen/logrus"
)

const accountsSchema = `
-- Stores data about accounts.
CREATE TABLE IF NOT EXISTS account_accounts (
    -- The Matrix user ID localpart for this account
    localpart TEXT NOT NULL PRIMARY KEY,
    -- When this account was first created, as a unix timestamp (ms resolution).
    created_ts BIGINT NOT NULL,
    -- The password hash for this account. Can be NULL if this is a passwordless account.
    password_hash TEXT,
    -- Identifies which application service this account belongs to, if any.
    appservice_id TEXT,
    -- If the account is currently active
    is_deactivated BOOLEAN DEFAULT 0
    -- TODO:
    -- is_guest, is_admin, upgraded_ts, devices, any email reset stuff?
);
`

const insertAccountSQL = "" +
	"INSERT INTO account_accounts(localpart, created_ts, password_hash, appservice_id) VALUES ($1, $2, $3, $4)"

const updatePasswordSQL = "" +
	"UPDATE account_accounts SET password_hash = $1 WHERE localpart = $2"

const deactivateAccountSQL = "" +
	"UPDATE account_accounts SET is_deactivated = 1 WHERE localpart = $1"

const selectAccountByLocalpartSQL = "" +
	"SELECT localpart, appservice_id FROM account_accounts WHERE localpart = $1"

const selectPasswordHashSQL = "" +
	"SELECT password_hash FROM account_accounts WHERE localpart = $1 AND is_deactivated = 0"

const selectNewNumericLocalpartSQL = "" +
	"SELECT COUNT(localpart) FROM account_accounts"

type accountsStatements struct {
	db                            *sql.DB
	insertAccountStmt             *sql.Stmt
	updatePasswordStmt            *sql.Stmt
	deactivateAccountStmt         *sql.Stmt
	selectAccountByLocalpartStmt  *sql.Stmt
	selectPasswordHashStmt        *sql.Stmt
	selectNewNumericLocalpartStmt *sql.Stmt
	serverName                    gomatrixserverlib.ServerName
}

func (s *accountsStatements) execSchema(db *sql.DB) error {
	_, err := db.Exec(accountsSchema)
	return err
}

func (s *accountsStatements) prepare(db *sql.DB, server gomatrixserverlib.ServerName) (err error) {
	s.db = db
	s.serverName = server
	return sqlutil.StatementList{
		{&s.insertAccountStmt, insertAccountSQL},
		{&s.updatePasswordStmt, updatePasswordSQL},
		{&s.deactivateAccountStmt, deactivateAccountSQL},
		{&s.selectAccountByLocalpartStmt, selectAccountByLocalpartSQL},
		{&s.selectPasswordHashStmt, selectPasswordHashSQL},
		{&s.selectNewNumericLocalpartStmt, selectNewNumericLocalpartSQL},
	}.Prepare(db)
}

// insertAccount creates a new account. 'hash' should be the password hash for this account. If it is missing,
// this account will be passwordless. Returns an error if this account already exists. Returns the account
// on success.
func (s *accountsStatements) insertAccount(
	ctx context.Context, txn *sql.Tx, localpart, hash, appserviceID string,
) (*api.Account, error) {
	createdTimeMS := time.Now().UnixNano() / 1000000
	stmt := s.insertAccountStmt

	var err error
	if appserviceID == "" {
		_, err = sqlutil.TxStmt(txn, stmt).ExecContext(ctx, localpart, createdTimeMS, hash, nil)
	} else {
		_, err = sqlutil.TxStmt(txn, stmt).ExecContext(ctx, localpart, createdTimeMS, hash, appserviceID)
	}
	if err != nil {
		return nil, err
	}

	return &api.Account{
		Localpart:    localpart,
		UserID:       userutil.MakeUserID(localpart, s.serverName),
		ServerName:   s.serverName,
		AppServiceID: appserviceID,
	}, nil
}

func (s *accountsStatements) updatePassword(
	ctx context.Context, localpart, passwordHash string,
) (err error) {
	_, err = s.updatePasswordStmt.ExecContext(ctx, passwordHash, localpart)
	return
}

func (s *accountsStatements) deactivateAccount(
	ctx context.Context, localpart string,
) (err error) {
	_, err = s.deactivateAccountStmt.ExecContext(ctx, localpart)
	return
}

func (s *accountsStatements) selectPasswordHash(
	ctx context.Context, localpart string,
) (hash string, err error) {
	err = s.selectPasswordHashStmt.QueryRowContext(ctx, localpart).Scan(&hash)
	return
}

func (s *accountsStatements) selectAccountByLocalpart(
	ctx context.Context, localpart string,
) (*api.Account, error) {
	var appserviceIDPtr sql.NullString
	var acc api.Account

	stmt := s.selectAccountByLocalpartStmt
	err := stmt.QueryRowContext(ctx, localpart).Scan(&acc.Localpart, &appserviceIDPtr)
	if err != nil {
		if err != sql.ErrNoRows {
			log.WithError(err).Error("Unable to retrieve user from the db")
		}
		return nil, err
	}
	if appserviceIDPtr.Valid {
		acc.AppServiceID = appserviceIDPtr.String
	}

	acc.UserID = userutil.MakeUserID(localpart, s.serverName)
	acc.ServerName = s.serverName

	return &acc, nil
}

func (s *accountsStatements) selectNewNumericLocalpart(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	stmt := s.selectNewNumericLocalpartStmt
	if txn != nil {
		stmt = sqlutil.TxStmt(txn, stmt)
	}
	err = stmt.QueryRowContext(ctx).Scan(&id)
	return
}
