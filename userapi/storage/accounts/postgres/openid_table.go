package postgres

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

const openIDTokenSchema = `
-- Stores data about openid tokens issued for accounts.
CREATE TABLE IF NOT EXISTS open_id_tokens (
	-- This is a hash the token value
	token_hash TEXT NOT NULL PRIMARY KEY,
    -- The Matrix user ID for this account
	localpart TEXT NOT NULL,
	-- When the token expires, as a unix timestamp (ms resolution).
	token_expires_ts BIGINT NOT NULL
);
`

const insertTokenSQL = "" +
	"INSERT INTO open_id_tokens(token_hash, localpart, token_expires_ts) VALUES ($1, $2, $3)"

const selectTokenSQL = "" +
	"SELECT localpart, token_expires_ts FROM open_id_tokens WHERE token_hash = $1"

type tokenStatements struct {
	insertTokenStmt *sql.Stmt
	selectTokenStmt *sql.Stmt
	serverName      gomatrixserverlib.ServerName
}

func (s *tokenStatements) prepare(db *sql.DB, server gomatrixserverlib.ServerName) (err error) {
	_, err = db.Exec(openIDTokenSchema)
	if err != nil {
		return
	}
	if s.insertTokenStmt, err = db.Prepare(insertTokenSQL); err != nil {
		return
	}
	if s.selectTokenStmt, err = db.Prepare(selectTokenSQL); err != nil {
		return
	}
	s.serverName = server
	return
}

// insertToken inserts a new OpenID Connect token to the DB.
// Returns new token, otherwise returns error if token hash already exists.
func (s *tokenStatements) insertToken(
	ctx context.Context,
	txn *sql.Tx,
	tokenHash, localpart string,
	expiresTimeMS int64,
) (err error) {
	stmt := sqlutil.TxStmt(txn, s.insertTokenStmt)
	_, err = stmt.ExecContext(ctx, tokenHash, localpart, expiresTimeMS)
	return
}

// selectOpenIDTokenAtrributesByTokenHash gets the attributes associated with an OpenID token from the DB
// Returns the existing token's attributes, or err if no token is found
func (s *tokenStatements) selectOpenIDTokenAtrributesByTokenHash(
	ctx context.Context,
	tokenHash string,
) (*api.OpenIDTokenAttributes, error) {
	var openIDTokenAttrs api.OpenIDTokenAttributes
	err := s.selectTokenStmt.QueryRowContext(ctx, tokenHash).Scan(
		&openIDTokenAttrs.UserID,
		&openIDTokenAttrs.ExpiresTS,
	)
	if err != nil {
		if err != sql.ErrNoRows {
			log.WithError(err).Error("Unable to retrieve token from the db")
		}
		return nil, err
	}

	return &openIDTokenAttrs, nil
}
