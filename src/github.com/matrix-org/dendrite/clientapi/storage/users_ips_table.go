package storage

import (
	"database/sql"
)

const userIPsSchema = `
CREATE TABLE IF NOT EXISTS user_ips (
	user_id TEXT NOT NULL,
	access_token TEXT NOT NULL,
	device_id TEXT,
	ip TEXT NOT NULL,
	user_agent TEXT NOT NULL,
	last_seen BIGINT NOT NULL
);

CREATE INDEX user_ips_user_ip ON user_ips(user_id, access_token, ip);
CREATE INDEX user_ips_device_id ON user_ips (user_id, device_id, last_seen);
`

type userIPStatements struct {
}

func (s *userIPStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(userIPsSchema)
	if err != nil {
		return
	}
	return
}
