package storage

import (
	"database/sql"
)

const accessTokensSchema = `
CREATE TABLE IF NOT EXISTS access_tokens (
	id BIGINT PRIMARY KEY,
	user_id TEXT NOT NULL,
	device_id TEXT,
	token TEXT NOT NULL,
	-- Timestamp (ms) when this access token was last used.
	last_used BIGINT,
	UNIQUE(token)
);

CREATE INDEX access_tokens_device_id ON access_tokens (user_id, device_id) ;
`

type accessTokenStatements struct {
}

func (s *accessTokenStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(accessTokensSchema)
	if err != nil {
		return
	}
	return
}
