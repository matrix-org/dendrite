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

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
)

// const profilesSchema = `
// -- Stores data about accounts profiles.
// CREATE TABLE IF NOT EXISTS account_profiles (
//     -- The Matrix user ID localpart for this account
//     localpart TEXT NOT NULL PRIMARY KEY,
//     -- The display name for this account
//     display_name TEXT,
//     -- The URL of the avatar for this account
//     avatar_url TEXT
// );
// `

// Profile represents the profile for a Matrix account.
type ProfileCosmos struct {
	Localpart   string `json:"local_part"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

type ProfileCosmosData struct {
	Id        string        `json:"id"`
	Pk        string        `json:"_pk"`
	Cn        string        `json:"_cn"`
	ETag      string        `json:"_etag"`
	Timestamp int64         `json:"_ts"`
	Profile   ProfileCosmos `json:"mx_userapi_profile"`
}

type profilesStatements struct {
	db *Database
	// insertProfileStmt            *sql.Stmt
	selectProfileByLocalpartStmt string
	// setAvatarURLStmt             *sql.Stmt
	// setDisplayNameStmt           *sql.Stmt
	selectProfilesBySearchStmt string
	tableName                  string
}

func mapFromProfile(db ProfileCosmos) authtypes.Profile {
	return authtypes.Profile{
		AvatarURL:   db.AvatarURL,
		DisplayName: db.DisplayName,
		Localpart:   db.Localpart,
	}
}

func mapToProfile(api authtypes.Profile) ProfileCosmos {
	return ProfileCosmos{
		AvatarURL:   api.AvatarURL,
		DisplayName: api.DisplayName,
		Localpart:   api.Localpart,
	}
}

func (s *profilesStatements) prepare(db *Database) (err error) {
	s.db = db
	s.selectProfileByLocalpartStmt = "select * from c where c._cn = @x1 and c.mx_userapi_profile.local_part = @x2"
	s.selectProfilesBySearchStmt = "select top @x3 * from c where c._cn = @x1 and contains(c.mx_userapi_profile.local_part, @x2)"
	s.tableName = "account_profiles"
	return
}

func getProfile(s *profilesStatements, ctx context.Context, config cosmosdbapi.Tenant, pk string, docId string) (*ProfileCosmosData, error) {
	response := ProfileCosmosData{}
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

func setProfile(s *profilesStatements, ctx context.Context, config cosmosdbapi.Tenant, pk string, profile ProfileCosmosData) (*ProfileCosmosData, error) {
	var optionsReplace = cosmosdbapi.GetReplaceDocumentOptions(pk, profile.ETag)
	var _, _, ex = cosmosdbapi.GetClient(s.db.connection).ReplaceDocument(
		ctx,
		config.DatabaseName,
		config.TenantName,
		profile.Id,
		&profile,
		optionsReplace)
	return &profile, ex
}

func (s *profilesStatements) insertProfile(
	ctx context.Context, localpart string,
) error {

	// 	"INSERT INTO account_profiles(localpart, display_name, avatar_url) VALUES ($1, $2, $3)"
	var result = &authtypes.Profile{
		Localpart: localpart,
	}

	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.profiles.tableName)

	var dbData = ProfileCosmosData{
		Id:        cosmosdbapi.GetDocumentId(config.TenantName, dbCollectionName, result.Localpart),
		Cn:        dbCollectionName,
		Pk:        cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName),
		Timestamp: time.Now().Unix(),
		Profile:   mapToProfile(*result),
	}

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	var _, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		config.DatabaseName,
		config.TenantName,
		dbData,
		options)

	return err
}

func (s *profilesStatements) selectProfileByLocalpart(
	ctx context.Context, localpart string,
) (*authtypes.Profile, error) {

	// "SELECT localpart, display_name, avatar_url FROM account_profiles WHERE localpart = $1"
	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.profiles.tableName)
	var pk = cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName)
	response := []ProfileCosmosData{}
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": localpart,
	}
	var options = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(s.selectProfileByLocalpartStmt, params)
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
		return nil, errors.New(fmt.Sprintf("Localpart %s not found", len(response)))
	}

	if len(response) != 1 {
		return nil, errors.New(fmt.Sprintf("Localpart %s has multiple entries", len(response)))
	}

	var result = mapFromProfile(response[0].Profile)
	return &result, nil
}

func (s *profilesStatements) setAvatarURL(
	ctx context.Context, localpart string, avatarURL string,
) (err error) {

	// "UPDATE account_profiles SET avatar_url = $1 WHERE localpart = $2"
	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.profiles.tableName)
	var pk = cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName)
	var docId = cosmosdbapi.GetDocumentId(config.TenantName, dbCollectionName, localpart)

	var response, exGet = getProfile(s, ctx, config, pk, docId)
	if exGet != nil {
		return exGet
	}

	response.Profile.AvatarURL = avatarURL

	var _, exReplace = setProfile(s, ctx, config, pk, *response)
	if exReplace != nil {
		return exReplace
	}
	return
}

func (s *profilesStatements) setDisplayName(
	ctx context.Context, localpart string, displayName string,
) (err error) {

	// "UPDATE account_profiles SET display_name = $1 WHERE localpart = $2"
	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.profiles.tableName)
	var pk = cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName)
	var docId = cosmosdbapi.GetDocumentId(config.TenantName, dbCollectionName, localpart)
	var response, exGet = getProfile(s, ctx, config, pk, docId)
	if exGet != nil {
		return exGet
	}

	response.Profile.DisplayName = displayName

	var _, exReplace = setProfile(s, ctx, config, pk, *response)
	if exReplace != nil {
		return exReplace
	}
	return
}

func (s *profilesStatements) selectProfilesBySearch(
	ctx context.Context, searchString string, limit int,
) ([]authtypes.Profile, error) {
	var profiles []authtypes.Profile

	// "SELECT localpart, display_name, avatar_url FROM account_profiles WHERE localpart LIKE $1 OR display_name LIKE $1 LIMIT $2"
	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.profiles.tableName)
	var pk = cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName)
	response := []ProfileCosmosData{}
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": searchString,
		"@x3": limit,
	}
	var options = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(s.selectProfilesBySearchStmt, params)
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

	for i := 0; i < len(response); i++ {
		var responseData = response[i]
		profiles = append(profiles, mapFromProfile(responseData.Profile))
	}

	return profiles, nil
}
