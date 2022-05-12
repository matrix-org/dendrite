package postgres

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

const openIDTokenSchema = `
-- Stores data about openid tokens issued for accounts.
CREATE TABLE IF NOT EXISTS open_id_tokens (
	-- The value of the token issued to a user
	token TEXT NOT NULL PRIMARY KEY,
    -- The Matrix user ID for this account
	localpart TEXT NOT NULL,
	-- When the token expires, as a unix timestamp (ms resolution).
	token_expires_at_ms BIGINT NOT NULL
);
`

const insertOpenIDTokenSQL = "" +
	"INSERT INTO open_id_tokens(token, localpart, token_expires_at_ms) VALUES ($1, $2, $3)"

const selectOpenIDTokenSQL = "" +
	"SELECT localpart, token_expires_at_ms FROM open_id_tokens WHERE token = $1"

type openIDTokenStatements struct {
	insertTokenStmt *sql.Stmt
	selectTokenStmt *sql.Stmt
	serverName      gomatrixserverlib.ServerName
}

func NewPostgresOpenIDTable(db *sql.DB, serverName gomatrixserverlib.ServerName) (tables.OpenIDTable, error) {
	s := &openIDTokenStatements{
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
	token, localpart string,
	expiresAt gomatrixserverlib.Timestamp,
) (err error) {
	stmt := sqlutil.TxStmt(txn, s.insertTokenStmt)
	_, err = stmt.ExecContext(ctx, token, localpart, expiresAt)
	return
}

// selectOpenIDTokenAtrributes gets the attributes associated with an OpenID token from the DB
// Returns the existing token's attributes, or err if no token is found
func (s *openIDTokenStatements) SelectOpenIDTokenAtrributes(
	ctx context.Context,
	token string,
) (*api.OpenIDTokenAttributes, error) {
	var openIDTokenAttrs api.OpenIDTokenAttributes
	err := s.selectTokenStmt.QueryRowContext(ctx, token).Scan(
		&openIDTokenAttrs.UserID,
		&openIDTokenAttrs.ExpiresAt,
	)
	if err != nil {
		if err != sql.ErrNoRows {
			log.WithError(err).Error("Unable to retrieve token from the db")
		}
		return nil, err
	}

	return &openIDTokenAttrs, nil
}
