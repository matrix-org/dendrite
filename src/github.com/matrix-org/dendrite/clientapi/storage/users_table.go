package storage

import (
	"database/sql"
)

const usersSchema = `
CREATE TABLE IF NOT EXISTS users (
	user_id TEXT NOT NULL,
	-- bcrypt hash of the users password. Can be null for passwordless users like
	-- application service users.
	password_hash TEXT,
	-- Timestamp (ms) when this user was registered on the server.
	created_at BIGINT,
	-- The ID of the application service which created this user, if applicable.
	appservice_id TEXT,
	-- Flag which if set indicates this user is a server administrator.
	is_admin SMALLINT DEFAULT 0 NOT NULL,
	-- Flag which if set indicates this user is a guest.
	is_guest SMALLINT DEFAULT 0 NOT NULL,
	UNIQUE(user_id)
);
`

type usersStatements struct {
}

func (s *usersStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(usersSchema)
	if err != nil {
		return
	}
	return
}
