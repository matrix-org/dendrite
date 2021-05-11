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

type AccountCosmos struct {
	UserID        string                       `json:"user_id"`
	Localpart     string                       `json:"local_part"`
	ServerName    gomatrixserverlib.ServerName `json:"server_name"`
	AppServiceID  string                       `json:"app_service_id"`
	IsDeactivated bool                         `json:"is_deactivated"`
	PasswordHash  string                       `json:"password_hash"`
	Created       int64                        `json:"created_ts"`
}

type AccountCosmosData struct {
	Id        string        `json:"id"`
	Pk        string        `json:"_pk"`
	Cn        string        `json:"_cn"`
	ETag      string        `json:"_etag"`
	Timestamp int64         `json:"_ts"`
	Account   AccountCosmos `json:"mx_userapi_account"`
}

type AccountCosmosUserCount struct {
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

func (s *accountsStatements) prepare(db *Database, server gomatrixserverlib.ServerName) (err error) {
	s.db = db
	s.selectPasswordHashStmt = "select * from c where c._cn = @x1 and c.mx_userapi_account.local_part = @x2 and c.mx_userapi_account.is_deactivated = false"
	s.selectAccountByLocalpartStmt = "select * from c where c._cn = @x1 and c.mx_userapi_account.local_part = @x2"
	s.selectNewNumericLocalpartStmt = "select count(c._ts) as usercount from c where c._cn = @x1"
	s.tableName = "account_accounts"
	s.serverName = server
	return
}

func getAccount(s *accountsStatements, ctx context.Context, config cosmosdbapi.Tenant, pk string, docId string) (*AccountCosmosData, error) {
	response := AccountCosmosData{}
	var optionsGet = cosmosdbapi.GetGetDocumentOptions(pk)
	var _, ex = cosmosdbapi.GetClient(s.db.connection).GetDocument(
		ctx,
		config.DatabaseName,
		config.TenantName,
		docId,
		optionsGet,
		&response)
	return &response, ex
}

func setAccount(s *accountsStatements, ctx context.Context, config cosmosdbapi.Tenant, pk string, account AccountCosmosData) (*AccountCosmosData, error) {
	response := AccountCosmosData{}
	var optionsReplace = cosmosdbapi.GetReplaceDocumentOptions(pk, account.ETag)
	var _, _, ex = cosmosdbapi.GetClient(s.db.connection).ReplaceDocument(
		ctx,
		config.DatabaseName,
		config.TenantName,
		account.Id,
		&account,
		optionsReplace)
	return &response, ex
}

func mapFromAccount(db AccountCosmos) api.Account {
	return api.Account{
		AppServiceID: db.AppServiceID,
		Localpart:    db.Localpart,
		ServerName:   db.ServerName,
		UserID:       db.UserID,
	}
}

func mapToAccount(api api.Account) AccountCosmos {
	return AccountCosmos{
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

	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.accounts.tableName)

	var dbData = AccountCosmosData{
		Id:        cosmosdbapi.GetDocumentId(config.TenantName, dbCollectionName, result.Localpart),
		Cn:        dbCollectionName,
		Pk:        cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName),
		Timestamp: time.Now().Unix(),
		Account:   data,
	}

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	var _, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		config.DatabaseName,
		config.TenantName,
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
	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.accounts.tableName)
	var docId = cosmosdbapi.GetDocumentId(config.TenantName, dbCollectionName, localpart)
	var pk = cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName)

	var response, exGet = getAccount(s, ctx, config, pk, docId)
	if exGet != nil {
		return exGet
	}

	response.Account.PasswordHash = passwordHash

	var _, exReplace = setAccount(s, ctx, config, pk, *response)
	if exReplace != nil {
		return exReplace
	}
	return
}

func (s *accountsStatements) deactivateAccount(
	ctx context.Context, localpart string,
) (err error) {

	// "UPDATE account_accounts SET is_deactivated = 1 WHERE localpart = $1"
	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.accounts.tableName)
	var docId = cosmosdbapi.GetDocumentId(config.TenantName, dbCollectionName, localpart)
	var pk = cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName)

	var response, exGet = getAccount(s, ctx, config, pk, docId)
	if exGet != nil {
		return exGet
	}

	response.Account.IsDeactivated = true

	var _, exReplace = setAccount(s, ctx, config, pk, *response)
	if exReplace != nil {
		return exReplace
	}
	return
}

func (s *accountsStatements) selectPasswordHash(
	ctx context.Context, localpart string,
) (hash string, err error) {

	// "SELECT password_hash FROM account_accounts WHERE localpart = $1 AND is_deactivated = 0"
	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.accounts.tableName)
	var pk = cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName)
	response := []AccountCosmosData{}
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": localpart,
	}
	var options = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(s.selectPasswordHashStmt, params)
	var _, ex = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		config.DatabaseName,
		config.TenantName,
		query,
		&response,
		options)

	if ex != nil {
		return "", ex
	}

	if len(response) == 0 {
		return "", errors.New(fmt.Sprintf("Localpart %s not found", localpart))
	}

	if len(response) != 1 {
		return "", errors.New(fmt.Sprintf("Localpart %s has multiple entries", localpart))
	}

	return response[0].Account.PasswordHash, nil
}

func (s *accountsStatements) selectAccountByLocalpart(
	ctx context.Context, localpart string,
) (*api.Account, error) {
	var acc api.Account

	// "SELECT localpart, appservice_id FROM account_accounts WHERE localpart = $1"
	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.accounts.tableName)
	var pk = cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName)
	response := []AccountCosmosData{}
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": localpart,
	}
	var options = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(s.selectAccountByLocalpartStmt, params)
	var _, ex = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		config.DatabaseName,
		config.TenantName,
		query,
		&response,
		options)

	if ex != nil {
		return nil, ex
	}

	if len(response) == 0 {
		return nil, nil
	}

	acc = mapFromAccount(response[0].Account)
	acc.UserID = userutil.MakeUserID(localpart, s.serverName)
	acc.ServerName = s.serverName

	return &acc, nil
}

func (s *accountsStatements) selectNewNumericLocalpart(
	ctx context.Context,
) (id int64, err error) {

	// 	"SELECT COUNT(localpart) FROM account_accounts"
	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.accounts.tableName)
	var pk = cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName)
	var response []AccountCosmosUserCount
	params := map[string]interface{}{
		"@x1": dbCollectionName,
	}
	var options = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(s.selectNewNumericLocalpartStmt, params)
	var _, ex = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		config.DatabaseName,
		config.TenantName,
		query,
		&response,
		options)

	if ex != nil {
		return -1, ex
	}

	return int64(response[0].UserCount), nil
}
