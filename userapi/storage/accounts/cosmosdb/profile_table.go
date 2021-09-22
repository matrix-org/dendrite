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

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

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
type profileCosmos struct {
	Localpart   string `json:"local_part"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

type profileCosmosData struct {
	cosmosdbapi.CosmosDocument
	Profile profileCosmos `json:"mx_userapi_profile"`
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

func (s *profilesStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *profilesStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func mapFromProfile(db profileCosmos) authtypes.Profile {
	return authtypes.Profile{
		AvatarURL:   db.AvatarURL,
		DisplayName: db.DisplayName,
		Localpart:   db.Localpart,
	}
}

func mapToProfile(api authtypes.Profile) profileCosmos {
	return profileCosmos{
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

func getProfile(s *profilesStatements, ctx context.Context, pk string, docId string) (*profileCosmosData, error) {
	response := profileCosmosData{}
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

func setProfile(s *profilesStatements, ctx context.Context, profile profileCosmosData) (*profileCosmosData, error) {
	var optionsReplace = cosmosdbapi.GetReplaceDocumentOptions(profile.Pk, profile.ETag)
	var _, _, ex = cosmosdbapi.GetClient(s.db.connection).ReplaceDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
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

	docId := localpart
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	var dbData = profileCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
		Profile:        mapToProfile(*result),
	}

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	var _, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData,
		options)

	return err
}

func (s *profilesStatements) selectProfileByLocalpart(
	ctx context.Context, localpart string,
) (*authtypes.Profile, error) {

	// "SELECT localpart, display_name, avatar_url FROM account_profiles WHERE localpart = $1"
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": localpart,
	}
	var rows []profileCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectProfileByLocalpartStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return nil, errors.New(fmt.Sprintf("Localpart %s not found", len(rows)))
	}

	if len(rows) != 1 {
		return nil, errors.New(fmt.Sprintf("Localpart %s has multiple entries", len(rows)))
	}

	var result = mapFromProfile(rows[0].Profile)
	return &result, nil
}

func (s *profilesStatements) setAvatarURL(
	ctx context.Context, localpart string, avatarURL string,
) (err error) {

	// "UPDATE account_profiles SET avatar_url = $1 WHERE localpart = $2"
	docId := localpart
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	var response, exGet = getProfile(s, ctx, s.getPartitionKey(), cosmosDocId)
	if exGet != nil {
		return exGet
	}

	response.Profile.AvatarURL = avatarURL

	var _, exReplace = setProfile(s, ctx, *response)
	if exReplace != nil {
		return exReplace
	}
	return
}

func (s *profilesStatements) setDisplayName(
	ctx context.Context, localpart string, displayName string,
) (err error) {

	// "UPDATE account_profiles SET display_name = $1 WHERE localpart = $2"
	docId := localpart
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)
	var response, exGet = getProfile(s, ctx, s.getPartitionKey(), cosmosDocId)
	if exGet != nil {
		return exGet
	}

	response.Profile.DisplayName = displayName

	var _, exReplace = setProfile(s, ctx, *response)
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
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": searchString,
		"@x3": limit,
	}
	var rows []profileCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectProfilesBySearchStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	for i := 0; i < len(rows); i++ {
		var responseData = rows[i]
		profiles = append(profiles, mapFromProfile(responseData.Profile))
	}

	return profiles, nil
}
