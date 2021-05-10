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

type AccountExtended struct {
	IsDeactivated bool   `json:"is_deactivated"`
	PasswordHash  string `json:"password_hash"`
	Created       int64  `json:"created_ts"`
}

type AccountCosmosData struct {
	Id             string          `json:"id"`
	Pk             string          `json:"_pk"`
	Cn             string          `json:"_cn"`
	ETag           string          `json:"_etag"`
	Timestamp      int64           `json:"_ts"`
	Object         api.Account     `json:"_object"`
	ObjectExtended AccountExtended `json:"_object_extended"`
}

type AccountCosmosUserCount struct {
	UserCount int64 `json:"usercount"`
}

type accountsStatements struct {
	db         *Database
	tableName  string
	serverName gomatrixserverlib.ServerName
}

func (s *accountsStatements) prepare(db *Database, server gomatrixserverlib.ServerName) (err error) {
	s.db = db
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

	var extended = AccountExtended{
		IsDeactivated: false,
		PasswordHash:  hash,
		Created:       createdTimeMS,
	}

	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.accounts.tableName)

	var dbData = AccountCosmosData{
		Id:             cosmosdbapi.GetDocumentId(config.TenantName, dbCollectionName, result.Localpart),
		Cn:             dbCollectionName,
		Pk:             cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName),
		Timestamp:      time.Now().Unix(),
		Object:         result,
		ObjectExtended: extended,
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

	response.ObjectExtended.PasswordHash = passwordHash

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

	response.ObjectExtended.IsDeactivated = true

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
	var selectPasswordHashCosmos = "select * from c where c._cn = @x1 and c._object.Localpart = @x2 and c._object_extended.is_deactivated = false"
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": localpart,
	}
	var options = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(selectPasswordHashCosmos, params)
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

	return response[0].ObjectExtended.PasswordHash, nil
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
	var selectPasswordHashCosmos = "select * from c where c._cn = @x1 and c._object.Localpart = @x2"
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": localpart,
	}
	var options = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(selectPasswordHashCosmos, params)
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

	acc = response[0].Object
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
	var selectCountCosmos = "select count(c._ts) as usercount from c where c._cn = @x1"
	params := map[string]interface{}{
		"@x1": dbCollectionName,
	}
	var options = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(selectCountCosmos, params)
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
