package cosmosdb

import (
	"context"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"

	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// const openIDTokenSchema = `
// -- Stores data about accounts.
// CREATE TABLE IF NOT EXISTS open_id_tokens (
// 	-- The value of the token issued to a user
// 	token TEXT NOT NULL PRIMARY KEY,
//     -- The Matrix user ID for this account
// 	localpart TEXT NOT NULL,
// 	-- When the token expires, as a unix timestamp (ms resolution).
// 	token_expires_at_ms BIGINT NOT NULL
// );
// `

// OpenIDToken represents an OpenID token
type OpenIDTokenCosmos struct {
	Token       string `json:"token"`
	UserID      string `json:"user_id"`
	ExpiresAtMS int64  `json:"expires_at"`
}

type OpenIdTokenCosmosData struct {
	cosmosdbapi.CosmosDocument
	OpenIdToken OpenIDTokenCosmos `json:"mx_userapi_openidtoken"`
}

type tokenStatements struct {
	db *Database
	// insertTokenStmt *sql.Stmt
	selectTokenStmt string
	tableName       string
	serverName      gomatrixserverlib.ServerName
}

func mapFromToken(db OpenIDTokenCosmos) api.OpenIDToken {
	return api.OpenIDToken{
		ExpiresAtMS: db.ExpiresAtMS,
		Token:       db.Token,
		UserID:      db.UserID,
	}
}

func mapToToken(api api.OpenIDToken) OpenIDTokenCosmos {
	return OpenIDTokenCosmos{
		ExpiresAtMS: api.ExpiresAtMS,
		Token:       api.Token,
		UserID:      api.UserID,
	}
}

func queryOpenIdToken(s *tokenStatements, ctx context.Context, qry string, params map[string]interface{}) ([]OpenIdTokenCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []OpenIdTokenCosmosData

	var optionsQry = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(qry, params)
	_, err := cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&response,
		optionsQry)

	if err != nil {
		return nil, err
	}
	return response, nil
}

func (s *tokenStatements) prepare(db *Database, server gomatrixserverlib.ServerName) (err error) {
	s.db = db
	s.selectTokenStmt = "select * from c where c._cn = @x1 and c.mx_userapi_openidtoken.token = @x2"
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
		UserID:      localpart,
		Token:       token,
		ExpiresAtMS: expiresAtMS,
	}

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.openIDTokens.tableName)

	docId := result.Token
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	var dbData = OpenIdTokenCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(dbCollectionName, s.db.cosmosConfig.TenantName, pk, cosmosDocId),
		OpenIdToken:    mapToToken(*result),
	}

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	var _, _, ex = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
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
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.openIDTokens.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": token,
	}
	response, err := queryOpenIdToken(s, ctx, s.selectTokenStmt, params)

	if err != nil {
		return nil, err
	}

	if len(response) == 0 {
		return nil, nil
	}

	var openIdToken = response[0].OpenIdToken
	openIDTokenAttrs = api.OpenIDTokenAttributes{
		UserID:      openIdToken.UserID,
		ExpiresAtMS: openIdToken.ExpiresAtMS,
	}
	return &openIDTokenAttrs, nil
}
