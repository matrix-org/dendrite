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
	"fmt"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/postgres/deltas"
	"github.com/matrix-org/dendrite/userapi/storage/tables"

	log "github.com/sirupsen/logrus"
)

const accountsSchema = `
-- Stores data about accounts.
CREATE TABLE IF NOT EXISTS userapi_accounts (
    -- The Matrix user ID localpart for this account
    localpart TEXT NOT NULL,
	server_name TEXT NOT NULL,
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

CREATE UNIQUE INDEX IF NOT EXISTS userapi_accounts_idx ON userapi_accounts(localpart, server_name);
`

const insertAccountSQL = "" +
	"INSERT INTO userapi_accounts(localpart, server_name, created_ts, password_hash, appservice_id, account_type) VALUES ($1, $2, $3, $4, $5, $6)"

const updatePasswordSQL = "" +
	"UPDATE userapi_accounts SET password_hash = $1 WHERE localpart = $2 AND server_name = $3"

const deactivateAccountSQL = "" +
	"UPDATE userapi_accounts SET is_deactivated = TRUE WHERE localpart = $1 AND server_name = $2"

const selectAccountByLocalpartSQL = "" +
	"SELECT localpart, server_name, appservice_id, account_type FROM userapi_accounts WHERE localpart = $1 AND server_name = $2"

const selectPasswordHashSQL = "" +
	"SELECT password_hash FROM userapi_accounts WHERE localpart = $1 AND server_name = $2 AND is_deactivated = FALSE"

const selectNewNumericLocalpartSQL = "" +
	"SELECT COALESCE(MAX(localpart::bigint), 0) FROM userapi_accounts WHERE localpart ~ '^[0-9]{1,}$' AND server_name = $1"

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
	m := sqlutil.NewMigrator(db)
	m.AddMigrations([]sqlutil.Migration{
		{
			Version: "userapi: add is active",
			Up:      deltas.UpIsActive,
			Down:    deltas.DownIsActive,
		},
		{
			Version: "userapi: add account type",
			Up:      deltas.UpAddAccountType,
			Down:    deltas.DownAddAccountType,
		},
	}...)
	err = m.Up(context.Background())
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
	ctx context.Context, txn *sql.Tx,
	localpart string, serverName gomatrixserverlib.ServerName,
	hash, appserviceID string, accountType api.AccountType,
) (*api.Account, error) {
	createdTimeMS := time.Now().UnixNano() / 1000000
	stmt := sqlutil.TxStmt(txn, s.insertAccountStmt)

	var err error
	if accountType != api.AccountTypeAppService {
		_, err = stmt.ExecContext(ctx, localpart, serverName, createdTimeMS, hash, nil, accountType)
	} else {
		_, err = stmt.ExecContext(ctx, localpart, serverName, createdTimeMS, hash, appserviceID, accountType)
	}
	if err != nil {
		return nil, fmt.Errorf("insertAccountStmt: %w", err)
	}

	return &api.Account{
		Localpart:    localpart,
		UserID:       userutil.MakeUserID(localpart, serverName),
		ServerName:   serverName,
		AppServiceID: appserviceID,
		AccountType:  accountType,
	}, nil
}

func (s *accountsStatements) UpdatePassword(
	ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName,
	passwordHash string,
) (err error) {
	_, err = s.updatePasswordStmt.ExecContext(ctx, passwordHash, localpart, serverName)
	return
}

func (s *accountsStatements) DeactivateAccount(
	ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName,
) (err error) {
	_, err = s.deactivateAccountStmt.ExecContext(ctx, localpart, serverName)
	return
}

func (s *accountsStatements) SelectPasswordHash(
	ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName,
) (hash string, err error) {
	err = s.selectPasswordHashStmt.QueryRowContext(ctx, localpart, serverName).Scan(&hash)
	return
}

func (s *accountsStatements) SelectAccountByLocalpart(
	ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName,
) (*api.Account, error) {
	var appserviceIDPtr sql.NullString
	var acc api.Account

	stmt := s.selectAccountByLocalpartStmt
	err := stmt.QueryRowContext(ctx, localpart, serverName).Scan(&acc.Localpart, &acc.ServerName, &appserviceIDPtr, &acc.AccountType)
	if err != nil {
		if err != sql.ErrNoRows {
			log.WithError(err).Error("Unable to retrieve user from the db")
		}
		return nil, err
	}
	if appserviceIDPtr.Valid {
		acc.AppServiceID = appserviceIDPtr.String
	}

	acc.UserID = userutil.MakeUserID(acc.Localpart, acc.ServerName)
	return &acc, nil
}

func (s *accountsStatements) SelectNewNumericLocalpart(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName,
) (id int64, err error) {
	stmt := s.selectNewNumericLocalpartStmt
	if txn != nil {
		stmt = sqlutil.TxStmt(txn, stmt)
	}
	err = stmt.QueryRowContext(ctx, serverName).Scan(&id)
	return id + 1, err
}
