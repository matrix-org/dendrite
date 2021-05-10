package cosmosdb

import (
	"time"
	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"context"

	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

const openIDTokenSchema = `
-- Stores data about accounts.
CREATE TABLE IF NOT EXISTS open_id_tokens (
	-- The value of the token issued to a user
	token TEXT NOT NULL PRIMARY KEY,
    -- The Matrix user ID for this account
	localpart TEXT NOT NULL,
	-- When the token expires, as a unix timestamp (ms resolution).
	token_expires_at_ms BIGINT NOT NULL
);
`
type OpenIdTokenCosmosData struct {
	Id string `json:"id"`
	Pk string `json:"_pk"`
	Cn string `json:"_cn"`
	ETag string `json:"_etag"`
	Timestamp int64 `json:"_ts"`
	Object *api.OpenIDToken `json:"_object"`
}

type tokenStatements struct {
	db              	*Database
	tableName	 		string 
	serverName      	gomatrixserverlib.ServerName
}

func (s *tokenStatements) prepare(db *Database, server gomatrixserverlib.ServerName) (err error) {
	s.db = db
	s.tableName = "open_id_tokens"
	s.serverName = server
	return
}

// insertToken inserts a new OpenID Connect token to the DB.
// Returns new token, otherwise returns error if the token already exists.
func (s *tokenStatements) insertToken(
	ctx context.Context,
	token, localpart string,
	expiresAtMS int64,
) (err error) {

	// "INSERT INTO open_id_tokens(token, localpart, token_expires_at_ms) VALUES ($1, $2, $3)"
	var result = &api.OpenIDToken{
		UserID:				localpart,
		Token:				token,
		ExpiresAtMS:		expiresAtMS,	
	}

	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.openIDTokens.tableName)

	var dbData = OpenIdTokenCosmosData{
		Id: cosmosdbapi.GetDocumentId(config.TenantName, dbCollectionName, result.Token),
		Cn: dbCollectionName,
		Pk: cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName),
		Timestamp: time.Now().Unix(),
		Object: result,
	}

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	var _, _, ex = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
			ctx, 
			config.DatabaseName, 
			config.TenantName, 
			dbData, 
			options)

	if ex != nil {
		return ex
	}

	return
}

// selectOpenIDTokenAtrributes gets the attributes associated with an OpenID token from the DB
// Returns the existing token's attributes, or err if no token is found
func (s *tokenStatements) selectOpenIDTokenAtrributes(
	ctx context.Context,
	token string,
) (*api.OpenIDTokenAttributes, error) {
	var openIDTokenAttrs api.OpenIDTokenAttributes

	// "SELECT localpart, token_expires_at_ms FROM open_id_tokens WHERE token = $1"
	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.openIDTokens.tableName)
	var pk = cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName)
	response := []OpenIdTokenCosmosData{}
	var selectOpenIdTokenCosmos = "select * from c where c._cn = @x1 and c._object.Token = @x2"
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": token,
	}
	var options = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(selectOpenIdTokenCosmos, params)
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

	if(len(response) == 0) {
		return nil, nil
	}

	var openIdToken = response[0].Object
	openIDTokenAttrs = api.OpenIDTokenAttributes{
		UserID: openIdToken.UserID,
		ExpiresAtMS: openIdToken.ExpiresAtMS,
	}
	return &openIDTokenAttrs, nil
}
