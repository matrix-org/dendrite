package sqlite3

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

const openIDTokenSchema = `
-- Stores data about accounts.
CREATE TABLE IF NOT EXISTS userapi_openid_tokens (
	-- The value of the token issued to a user
	token TEXT NOT NULL PRIMARY KEY,
    -- The Matrix user ID for this account
	localpart TEXT NOT NULL,
	server_name TEXT NOT NULL,
	-- When the token expires, as a unix timestamp (ms resolution).
	token_expires_at_ms BIGINT NOT NULL
);
`

const insertOpenIDTokenSQL = "" +
	"INSERT INTO userapi_openid_tokens(token, localpart, server_name, token_expires_at_ms) VALUES ($1, $2, $3, $4)"

const selectOpenIDTokenSQL = "" +
	"SELECT localpart, server_name, token_expires_at_ms FROM userapi_openid_tokens WHERE token = $1"

type openIDTokenStatements struct {
	db              *sql.DB
	insertTokenStmt *sql.Stmt
	selectTokenStmt *sql.Stmt
	serverName      gomatrixserverlib.ServerName
}

func NewSQLiteOpenIDTable(db *sql.DB, serverName gomatrixserverlib.ServerName) (tables.OpenIDTable, error) {
	s := &openIDTokenStatements{
		db:         db,
		serverName: serverName,
	}
	_, err := db.Exec(openIDTokenSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.insertTokenStmt, insertOpenIDTokenSQL},
		{&s.selectTokenStmt, selectOpenIDTokenSQL},
	}.Prepare(db)
}

// insertToken inserts a new OpenID Connect token to the DB.
// Returns new token, otherwise returns error if the token already exists.
func (s *openIDTokenStatements) InsertOpenIDToken(
	ctx context.Context,
	txn *sql.Tx,
	token, localpart string, serverName gomatrixserverlib.ServerName,
	expiresAtMS int64,
) (err error) {
	stmt := sqlutil.TxStmt(txn, s.insertTokenStmt)
	_, err = stmt.ExecContext(ctx, token, localpart, serverName, expiresAtMS)
	return
}

// selectOpenIDTokenAtrributes gets the attributes associated with an OpenID token from the DB
// Returns the existing token's attributes, or err if no token is found
func (s *openIDTokenStatements) SelectOpenIDTokenAtrributes(
	ctx context.Context,
	token string,
) (*api.OpenIDTokenAttributes, error) {
	var openIDTokenAttrs api.OpenIDTokenAttributes
	var localpart string
	var serverName gomatrixserverlib.ServerName
	err := s.selectTokenStmt.QueryRowContext(ctx, token).Scan(
		&localpart, &serverName,
		&openIDTokenAttrs.ExpiresAtMS,
	)
	openIDTokenAttrs.UserID = fmt.Sprintf("@%s:%s", localpart, serverName)
	if err != nil {
		if err != sql.ErrNoRows {
			log.WithError(err).Error("Unable to retrieve token from the db")
		}
		return nil, err
	}

	return &openIDTokenAttrs, nil
}
