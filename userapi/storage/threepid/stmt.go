package threepid

import (
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
)

const sessionsSchema = `
-- This sequence is used for automatic allocation of session_id.
-- CREATE SEQUENCE IF NOT EXISTS threepid_session_id_seq START 1;

-- Stores data about devices.
CREATE TABLE IF NOT EXISTS threepid_sessions (
    sid VARCHAR(255) PRIMARY KEY,
    client_secret VARCHAR(255),
    threepid TEXT ,
    token VARCHAR(255) ,
    next_link TEXT,
    validated_at_ts BIGINT,
    validated BOOLEAN,
    send_attempt INT
);

CREATE UNIQUE INDEX IF NOT EXISTS threepid_sessions_threepids
    ON threepid_sessions (threepid, client_secret)
`

const (
	insertSessionSQL = "" +
		"INSERT INTO threepid_sessions (sid, client_secret, threepid, token, next_link, send_attempt, validated_at_ts, validated)" +
		"VALUES ($1, $2, $3, $4, $5, $6, $7, $8)"
	selectSessionSQL = "" +
		"SELECT client_secret, threepid, token, next_link, validated, validated_at_ts, send_attempt FROM threepid_sessions WHERE sid == $1"
	selectSessionByThreePidAndCLientSecretSQL = "" +
		"SELECT sid, token, next_link, validated, validated_at_ts, send_attempt FROM threepid_sessions WHERE threepid == $1 AND client_secret == $2"
	deleteSessionSQL = "" +
		"DELETE FROM threepid_sessions WHERE sid = $1"
	validateSessionSQL = "" +
		"UPDATE threepid_sessions SET validated = $1, validated_at_ts = $2 WHERE sid = $3"
	updateSendAttemptNextLinkSQL = "" +
		"UPDATE threepid_sessions SET send_attempt = send_attempt + 1, next_link = $1 WHERE sid = $2"
)

type sessionStatements struct {
	db                                         *sql.DB
	writer                                     sqlutil.Writer
	insertSessionStmt                          *sql.Stmt
	selectSessionStmt                          *sql.Stmt
	selectSessionByThreePidAndCLientSecretStmt *sql.Stmt
	deleteSessionStmt                          *sql.Stmt
	validateSessionStmt                        *sql.Stmt
	updateSendAttemptNextLinkStmt              *sql.Stmt
}

func (s *sessionStatements) prepare() (err error) {
	if s.insertSessionStmt, err = s.db.Prepare(insertSessionSQL); err != nil {
		return
	}
	if s.selectSessionStmt, err = s.db.Prepare(selectSessionSQL); err != nil {
		return
	}
	if s.selectSessionByThreePidAndCLientSecretStmt, err = s.db.Prepare(selectSessionByThreePidAndCLientSecretSQL); err != nil {
		return
	}
	if s.deleteSessionStmt, err = s.db.Prepare(deleteSessionSQL); err != nil {
		return
	}
	if s.validateSessionStmt, err = s.db.Prepare(validateSessionSQL); err != nil {
		return
	}
	if s.updateSendAttemptNextLinkStmt, err = s.db.Prepare(updateSendAttemptNextLinkSQL); err != nil {
		return
	}
	return
}

func (s *sessionStatements) execSchema(db *sql.DB) error {
	_, err := db.Exec(sessionsSchema)
	return err
}
