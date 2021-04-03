package sqlite3

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
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

const insertTokenSQL = "" +
	"INSERT INTO open_id_tokens(token, localpart, token_expires_at_ms) VALUES ($1, $2, $3)"

const selectTokenSQL = "" +
	"SELECT localpart, token_expires_at_ms FROM open_id_tokens WHERE token = $1"

type tokenStatements struct {
	db              *sql.DB
	insertTokenStmt *sql.Stmt
	selectTokenStmt *sql.Stmt
	serverName      gomatrixserverlib.ServerName
}

func (s *tokenStatements) prepare(db *sql.DB, server gomatrixserverlib.ServerName) (err error) {
	s.db = db
	_, err = db.Exec(openIDTokenSchema)
	if err != nil {
		return err
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
// Returns new token, otherwise returns error if the token already exists.
func (s *tokenStatements) insertToken(
	ctx context.Context,
	txn *sql.Tx,
	token, localpart string,
	expiresAtMS int64,
) (err error) {
	stmt := sqlutil.TxStmt(txn, s.insertTokenStmt)
	_, err = stmt.ExecContext(ctx, token, localpart, expiresAtMS)
	return
}

// selectOpenIDTokenAtrributes gets the attributes associated with an OpenID token from the DB
// Returns the existing token's attributes, or err if no token is found
func (s *tokenStatements) selectOpenIDTokenAtrributes(
	ctx context.Context,
	token string,
) (*api.OpenIDTokenAttributes, error) {
	var openIDTokenAttrs api.OpenIDTokenAttributes
	err := s.selectTokenStmt.QueryRowContext(ctx, token).Scan(
		&openIDTokenAttrs.UserID,
		&openIDTokenAttrs.ExpiresAtMS,
	)
	if err != nil {
		if err != sql.ErrNoRows {
			log.WithError(err).Error("Unable to retrieve token from the db")
		}
		return nil, err
	}

	return &openIDTokenAttrs, nil
}
