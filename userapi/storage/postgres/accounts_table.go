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

package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/tables"

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
    is_deactivated BOOLEAN DEFAULT FALSE,
	-- The account_type (user = 1, guest = 2, admin = 3, appservice = 4)
	account_type SMALLINT NOT NULL
    -- TODO:
    -- upgraded_ts, devices, any email reset stuff?
);
`

const insertAccountSQL = "" +
	"INSERT INTO account_accounts(localpart, created_ts, password_hash, appservice_id, account_type) VALUES ($1, $2, $3, $4, $5)"

const updatePasswordSQL = "" +
	"UPDATE account_accounts SET password_hash = $1 WHERE localpart = $2"

const deactivateAccountSQL = "" +
	"UPDATE account_accounts SET is_deactivated = TRUE WHERE localpart = $1"

const selectAccountByLocalpartSQL = "" +
	"SELECT localpart, appservice_id, account_type FROM account_accounts WHERE localpart = $1"

const selectPasswordHashSQL = "" +
	"SELECT password_hash FROM account_accounts WHERE localpart = $1 AND is_deactivated = FALSE"

const selectNewNumericLocalpartSQL = "" +
	"SELECT COALESCE(MAX(localpart::bigint), 0) FROM account_accounts WHERE localpart ~ '^[0-9]{1,}$'"

type accountsStatements struct {
	insertAccountStmt             *sql.Stmt
	updatePasswordStmt            *sql.Stmt
	deactivateAccountStmt         *sql.Stmt
	selectAccountByLocalpartStmt  *sql.Stmt
	selectPasswordHashStmt        *sql.Stmt
	selectNewNumericLocalpartStmt *sql.Stmt
	serverName                    gomatrixserverlib.ServerName
}

func NewPostgresAccountsTable(db *sql.DB, serverName gomatrixserverlib.ServerName) (tables.AccountsTable, error) {
	s := &accountsStatements{
		serverName: serverName,
	}
	_, err := db.Exec(accountsSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
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
func (s *accountsStatements) InsertAccount(
	ctx context.Context, txn *sql.Tx, localpart, hash, appserviceID string, accountType api.AccountType,
) (*api.Account, error) {
	createdTimeMS := time.Now().UnixNano() / 1000000
	stmt := sqlutil.TxStmt(txn, s.insertAccountStmt)

	var err error
	if accountType != api.AccountTypeAppService {
		_, err = stmt.ExecContext(ctx, localpart, createdTimeMS, hash, nil, accountType)
	} else {
		_, err = stmt.ExecContext(ctx, localpart, createdTimeMS, hash, appserviceID, accountType)
	}
	if err != nil {
		return nil, err
	}

	return &api.Account{
		Localpart:    localpart,
		UserID:       userutil.MakeUserID(localpart, s.serverName),
		ServerName:   s.serverName,
		AppServiceID: appserviceID,
		AccountType:  accountType,
	}, nil
}

func (s *accountsStatements) UpdatePassword(
	ctx context.Context, localpart, passwordHash string,
) (err error) {
	_, err = s.updatePasswordStmt.ExecContext(ctx, passwordHash, localpart)
	return
}

func (s *accountsStatements) DeactivateAccount(
	ctx context.Context, localpart string,
) (err error) {
	_, err = s.deactivateAccountStmt.ExecContext(ctx, localpart)
	return
}

func (s *accountsStatements) SelectPasswordHash(
	ctx context.Context, localpart string,
) (hash string, err error) {
	err = s.selectPasswordHashStmt.QueryRowContext(ctx, localpart).Scan(&hash)
	return
}

func (s *accountsStatements) SelectAccountByLocalpart(
	ctx context.Context, localpart string,
) (*api.Account, error) {
	var appserviceIDPtr sql.NullString
	var acc api.Account

	stmt := s.selectAccountByLocalpartStmt
	err := stmt.QueryRowContext(ctx, localpart).Scan(&acc.Localpart, &appserviceIDPtr, &acc.AccountType)
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

func (s *accountsStatements) SelectNewNumericLocalpart(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	stmt := s.selectNewNumericLocalpartStmt
	if txn != nil {
		stmt = sqlutil.TxStmt(txn, stmt)
	}
	err = stmt.QueryRowContext(ctx).Scan(&id)
	return id + 1, err
}
