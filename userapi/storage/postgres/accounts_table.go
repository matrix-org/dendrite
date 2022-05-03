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

	"github.com/matrix-org/dendrite/internal"
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
	account_type SMALLINT NOT NULL,
	-- The policy version this user has accepted
	policy_version TEXT,
   -- The policy version the user received from the server notices room
	policy_version_sent TEXT,
	server_notice_room_id TEXT
    -- TODO:
    -- upgraded_ts, devices, any email reset stuff?
);
`

const insertAccountSQL = "" +
	"INSERT INTO account_accounts(localpart, created_ts, password_hash, appservice_id, account_type, policy_version) VALUES ($1, $2, $3, $4, $5, $6)"

const updatePasswordSQL = "" +
	"UPDATE account_accounts SET password_hash = $1 WHERE localpart = $2"

const deactivateAccountSQL = "" +
	"UPDATE account_accounts SET is_deactivated = TRUE WHERE localpart = $1"

const selectAccountByLocalpartSQL = "" +
	"SELECT localpart, appservice_id, account_type FROM account_accounts WHERE localpart = $1"

const selectPasswordHashSQL = "" +
	"SELECT password_hash FROM account_accounts WHERE localpart = $1 AND is_deactivated = FALSE"

const selectNewNumericLocalpartSQL = "" +
	"SELECT COALESCE(MAX(localpart::integer), 0) FROM account_accounts WHERE localpart ~ '^[0-9]*$'"

const selectPrivacyPolicySQL = "" +
	"SELECT policy_version FROM account_accounts WHERE localpart = $1"

const batchSelectPrivacyPolicySQL = "" +
	"SELECT localpart FROM account_accounts WHERE (policy_version IS NULL OR policy_version <> $1) AND (policy_version_sent IS NULL OR policy_version_sent <> $1)"

const updatePolicyVersionSQL = "" +
	"UPDATE account_accounts SET policy_version = $1 WHERE localpart = $2"

const updatePolicyVersionServerNoticeSQL = "" +
	"UPDATE account_accounts SET policy_version_sent = $1 WHERE localpart = $2"

const selectServerNoticeRoomSQL = "" +
	"SELECT server_notice_room_id FROM account_accounts WHERE localpart = $1"

const updateServerNoticeRoomSQL = "" +
	"UPDATE account_accounts SET server_notice_room_id = $1 WHERE localpart = $2"

type accountsStatements struct {
	insertAccountStmt                   *sql.Stmt
	updatePasswordStmt                  *sql.Stmt
	deactivateAccountStmt               *sql.Stmt
	selectAccountByLocalpartStmt        *sql.Stmt
	selectPasswordHashStmt              *sql.Stmt
	selectNewNumericLocalpartStmt       *sql.Stmt
	selectPrivacyPolicyStmt             *sql.Stmt
	batchSelectPrivacyPolicyStmt        *sql.Stmt
	updatePolicyVersionStmt             *sql.Stmt
	updatePolicyVersionServerNoticeStmt *sql.Stmt
	selectServerNoticeRoomStmt          *sql.Stmt
	updateServerNoticeRoomStmt          *sql.Stmt
	serverName                          gomatrixserverlib.ServerName
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
		{&s.selectPrivacyPolicyStmt, selectPrivacyPolicySQL},
		{&s.batchSelectPrivacyPolicyStmt, batchSelectPrivacyPolicySQL},
		{&s.updatePolicyVersionStmt, updatePolicyVersionSQL},
		{&s.updatePolicyVersionServerNoticeStmt, updatePolicyVersionServerNoticeSQL},
		{&s.selectServerNoticeRoomStmt, selectServerNoticeRoomSQL},
		{&s.updateServerNoticeRoomStmt, updateServerNoticeRoomSQL},
	}.Prepare(db)
}

// insertAccount creates a new account. 'hash' should be the password hash for this account. If it is missing,
// this account will be passwordless. Returns an error if this account already exists. Returns the account
// on success.
func (s *accountsStatements) InsertAccount(
	ctx context.Context, txn *sql.Tx, localpart, hash, appserviceID, policyVersion string, accountType api.AccountType,
) (*api.Account, error) {
	createdTimeMS := time.Now().UnixNano() / 1000000
	stmt := sqlutil.TxStmt(txn, s.insertAccountStmt)

	_, err := stmt.ExecContext(ctx, localpart, createdTimeMS, hash, nil, accountType, policyVersion)
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

// selectPrivacyPolicy gets the current privacy policy a specific user accepted
func (s *accountsStatements) SelectPrivacyPolicy(
	ctx context.Context, txn *sql.Tx, localPart string,
) (policy string, err error) {
	stmt := s.selectPrivacyPolicyStmt
	if txn != nil {
		stmt = sqlutil.TxStmt(txn, stmt)
	}
	err = stmt.QueryRowContext(ctx, localPart).Scan(&policy)
	return
}

// batchSelectPrivacyPolicy queries all users which didn't accept the current policy version
func (s *accountsStatements) BatchSelectPrivacyPolicy(
	ctx context.Context, txn *sql.Tx, policyVersion string,
) (userIDs []string, err error) {
	stmt := s.batchSelectPrivacyPolicyStmt
	if txn != nil {
		stmt = sqlutil.TxStmt(txn, stmt)
	}
	rows, err := stmt.QueryContext(ctx, policyVersion)
	defer internal.CloseAndLogIfError(ctx, rows, "BatchSelectPrivacyPolicy: rows.close() failed")
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return userIDs, err
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, rows.Err()
}

// updatePolicyVersion sets the policy_version for a specific user
func (s *accountsStatements) UpdatePolicyVersion(
	ctx context.Context, txn *sql.Tx, policyVersion, localpart string, serverNotice bool,
) (err error) {
	stmt := s.updatePolicyVersionStmt
	if serverNotice {
		stmt = s.updatePolicyVersionServerNoticeStmt
	}
	if txn != nil {
		stmt = sqlutil.TxStmt(txn, stmt)
	}
	_, err = stmt.ExecContext(ctx, policyVersion, localpart)
	return err
}

// SelectServerNoticeRoomID queries the server notice room ID.
func (s *accountsStatements) SelectServerNoticeRoomID(
	ctx context.Context, txn *sql.Tx, localpart string,
) (roomID string, err error) {
	stmt := s.selectServerNoticeRoomStmt
	if txn != nil {
		stmt = sqlutil.TxStmt(txn, stmt)
	}

	roomIDNull := sql.NullString{}
	row := stmt.QueryRowContext(ctx, localpart)
	err = row.Scan(&roomIDNull)
	if err != nil {
		return "", err
	}
	if roomIDNull.Valid {
		return roomIDNull.String, nil
	}
	return "", nil
}

// UpdateServerNoticeRoomID sets the server notice room ID.
func (s *accountsStatements) UpdateServerNoticeRoomID(
	ctx context.Context, txn *sql.Tx, localpart, roomID string,
) (err error) {
	stmt := s.updateServerNoticeRoomStmt
	if txn != nil {
		stmt = sqlutil.TxStmt(txn, stmt)
	}
	_, err = stmt.ExecContext(ctx, roomID, localpart)
	return
}
