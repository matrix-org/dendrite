package shared

import (
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
)

const (
	insertSessionSQL = "" +
		"INSERT INTO threepid_sessions (client_secret, threepid, token, next_link, validated_at_ts, validated, send_attempt)" +
		"VALUES ($1, $2, $3, $4, $5, $6, $7)" +
		"RETURNING sid;"
	// selectSidSQL = "" +
	// 	"SELECT last_insert_rowid();"
	selectSessionSQL = "" +
		"SELECT client_secret, threepid, token, next_link, validated_at_ts, validated, send_attempt FROM threepid_sessions WHERE sid = $1"
	selectSessionByThreePidAndCLientSecretSQL = "" +
		"SELECT sid, token, next_link, validated_at_ts, validated, send_attempt FROM threepid_sessions WHERE threepid = $1 AND client_secret = $2"
	deleteSessionSQL = "" +
		"DELETE FROM threepid_sessions WHERE sid = $1"
	validateSessionSQL = "" +
		"UPDATE threepid_sessions SET validated = $1, validated_at_ts = $2 WHERE sid = $3"
	updateSendAttemptNextLinkSQL = "" +
		"UPDATE threepid_sessions SET send_attempt = send_attempt + 1, next_link = $1 WHERE sid = $2"
)

type ThreePidSessionStatements struct {
	insertSessionStmt *sql.Stmt
	// selectSidStmt                              *sql.Stmt
	selectSessionStmt                          *sql.Stmt
	selectSessionByThreePidAndCLientSecretStmt *sql.Stmt
	deleteSessionStmt                          *sql.Stmt
	validateSessionStmt                        *sql.Stmt
	updateSendAttemptNextLinkStmt              *sql.Stmt
}

func PrepareThreePidSessionsTable(db *sql.DB) (*ThreePidSessionStatements, error) {
	s := ThreePidSessionStatements{}
	return &s, sqlutil.StatementList{
		{&s.insertSessionStmt, insertSessionSQL},
		// {&s.selectSidStmt, selectSidSQL},
		{&s.selectSessionStmt, selectSessionSQL},
		{&s.selectSessionByThreePidAndCLientSecretStmt, selectSessionByThreePidAndCLientSecretSQL},
		{&s.deleteSessionStmt, deleteSessionSQL},
		{&s.validateSessionStmt, validateSessionSQL},
		{&s.updateSendAttemptNextLinkStmt, updateSendAttemptNextLinkSQL},
	}.Prepare(db)
}
