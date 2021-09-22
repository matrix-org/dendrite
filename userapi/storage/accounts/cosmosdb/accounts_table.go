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

package cosmosdb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// const accountsSchema = `
// -- Stores data about accounts.
// CREATE TABLE IF NOT EXISTS account_accounts (
//     -- The Matrix user ID localpart for this account
//     localpart TEXT NOT NULL PRIMARY KEY,
//     -- When this account was first created, as a unix timestamp (ms resolution).
//     created_ts BIGINT NOT NULL,
//     -- The password hash for this account. Can be NULL if this is a passwordless account.
//     password_hash TEXT,
//     -- Identifies which application service this account belongs to, if any.
//     appservice_id TEXT,
//     -- If the account is currently active
//     is_deactivated BOOLEAN DEFAULT 0
//     -- TODO:
//     -- is_guest, is_admin, upgraded_ts, devices, any email reset stuff?
// );
// `

type accountCosmos struct {
	UserID        string                       `json:"user_id"`
	Localpart     string                       `json:"local_part"`
	ServerName    gomatrixserverlib.ServerName `json:"server_name"`
	AppServiceID  string                       `json:"app_service_id"`
	IsDeactivated bool                         `json:"is_deactivated"`
	PasswordHash  string                       `json:"password_hash"`
	Created       int64                        `json:"created_ts"`
}

type accountCosmosData struct {
	cosmosdbapi.CosmosDocument
	Account accountCosmos `json:"mx_userapi_account"`
}

type accountCosmosUserCount struct {
	UserCount int64 `json:"usercount"`
}

type accountsStatements struct {
	db                            *Database
	selectAccountByLocalpartStmt  string
	selectPasswordHashStmt        string
	selectNewNumericLocalpartStmt string
	tableName                     string
	serverName                    gomatrixserverlib.ServerName
}

func (s *accountsStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *accountsStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func (s *accountsStatements) prepare(db *Database, server gomatrixserverlib.ServerName) (err error) {
	s.db = db
	s.selectPasswordHashStmt = "select * from c where c._cn = @x1 and c.mx_userapi_account.local_part = @x2 and c.mx_userapi_account.is_deactivated = false"
	s.selectAccountByLocalpartStmt = "select * from c where c._cn = @x1 and c.mx_userapi_account.local_part = @x2"
	s.selectNewNumericLocalpartStmt = "select count(c._ts) as usercount from c where c._cn = @x1"
	s.tableName = "account_accounts"
	s.serverName = server
	return
}

func getAccount(s *accountsStatements, ctx context.Context, pk string, docId string) (*accountCosmosData, error) {
	response := accountCosmosData{}
	err := cosmosdbapi.GetDocumentOrNil(
		s.db.connection,
		s.db.cosmosConfig,
		ctx,
		pk,
		docId,
		&response)

	if response.Id == "" {
		return nil, cosmosdbutil.ErrNoRows
	}

	return &response, err
}

func setAccount(s *accountsStatements, ctx context.Context, account accountCosmosData) (*accountCosmosData, error) {
	response := accountCosmosData{}
	var optionsReplace = cosmosdbapi.GetReplaceDocumentOptions(account.Pk, account.ETag)
	var _, _, ex = cosmosdbapi.GetClient(s.db.connection).ReplaceDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		account.Id,
		&account,
		optionsReplace)
	return &response, ex
}

func mapFromAccount(db accountCosmos) api.Account {
	return api.Account{
		AppServiceID: db.AppServiceID,
		Localpart:    db.Localpart,
		ServerName:   db.ServerName,
		UserID:       db.UserID,
	}
}

func mapToAccount(api api.Account) accountCosmos {
	return accountCosmos{
		AppServiceID: api.AppServiceID,
		Localpart:    api.Localpart,
		ServerName:   api.ServerName,
		UserID:       api.UserID,
	}
}

// insertAccount creates a new account. 'hash' should be the password hash for this account. If it is missing,
// this account will be passwordless. Returns an error if this account already exists. Returns the account
// on success.
func (s *accountsStatements) insertAccount(
	ctx context.Context, localpart, hash, appserviceID string,
) (*api.Account, error) {
	createdTimeMS := time.Now().UnixNano() / 1000000
	// stmt := s.insertAccountStmt

	var result = api.Account{
		Localpart:    localpart,
		UserID:       userutil.MakeUserID(localpart, s.serverName),
		ServerName:   s.serverName,
		AppServiceID: appserviceID,
	}

	//Add the extra properties not on the API
	var data = mapToAccount(result)
	data.Created = createdTimeMS
	data.PasswordHash = hash
	data.IsDeactivated = false

	docId := result.Localpart
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	var dbData = accountCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
		Account:        data,
	}

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	var _, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData,
		options)

	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (s *accountsStatements) updatePassword(
	ctx context.Context, localpart, passwordHash string,
) (err error) {

	// "UPDATE account_accounts SET password_hash = $1 WHERE localpart = $2"
	docId := localpart
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	var response, exGet = getAccount(s, ctx, s.getPartitionKey(), cosmosDocId)
	if exGet != nil {
		return exGet
	}

	response.Account.PasswordHash = passwordHash

	var _, exReplace = setAccount(s, ctx, *response)
	if exReplace != nil {
		return exReplace
	}
	return
}

func (s *accountsStatements) deactivateAccount(
	ctx context.Context, localpart string,
) (err error) {

	// "UPDATE account_accounts SET is_deactivated = 1 WHERE localpart = $1"
	docId := localpart
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	var response, exGet = getAccount(s, ctx, s.getPartitionKey(), cosmosDocId)
	if exGet != nil {
		return exGet
	}

	response.Account.IsDeactivated = true

	var _, exReplace = setAccount(s, ctx, *response)
	if exReplace != nil {
		return exReplace
	}
	return
}

func (s *accountsStatements) selectPasswordHash(
	ctx context.Context, localpart string,
) (hash string, err error) {

	// "SELECT password_hash FROM account_accounts WHERE localpart = $1 AND is_deactivated = 0"
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": localpart,
	}
	var rows []accountCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectPasswordHashStmt, params, &rows)

	if err != nil {
		return "", err
	}

	if len(rows) == 0 {
		return "", errors.New(fmt.Sprintf("Localpart %s not found", localpart))
	}

	if len(rows) != 1 {
		return "", errors.New(fmt.Sprintf("Localpart %s has multiple entries", localpart))
	}

	return rows[0].Account.PasswordHash, nil
}

func (s *accountsStatements) selectAccountByLocalpart(
	ctx context.Context, localpart string,
) (*api.Account, error) {
	var acc api.Account

	// "SELECT localpart, appservice_id FROM account_accounts WHERE localpart = $1"
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": localpart,
	}
	var rows []accountCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectAccountByLocalpartStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return nil, nil
	}

	acc = mapFromAccount(rows[0].Account)
	acc.UserID = userutil.MakeUserID(localpart, s.serverName)
	acc.ServerName = s.serverName

	return &acc, nil
}

func (s *accountsStatements) selectNewNumericLocalpart(
	ctx context.Context,
) (id int64, err error) {

	// 	"SELECT COUNT(localpart) FROM account_accounts"
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
	}

	var rows []accountCosmosUserCount
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectNewNumericLocalpartStmt, params, &rows)

	if err != nil {
		return -1, err
	}

	return int64(rows[0].UserCount), nil
}
