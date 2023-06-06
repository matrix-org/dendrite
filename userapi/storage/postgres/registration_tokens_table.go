package postgres

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
)

const registrationTokensSchema = `
CREATE TABLE IF NOT EXISTS userapi_registration_tokens (
	token TEXT PRIMARY KEY,
	pending BIGINT,
	completed BIGINT,
	uses_allowed BIGINT,
	expiry_time BIGINT
);
`

const selectTokenSQL = "" +
	"SELECT token FROM userapi_registration_tokens WHERE token = $1"

const insertTokenSQL = "" +
	"INSERT INTO userapi_registration_tokens (token, uses_allowed, expiry_time, pending, completed) VALUES ($1, $2, $3, $4, $5)"

type registrationTokenStatements struct {
	selectTokenStatement *sql.Stmt
	insertTokenStatment  *sql.Stmt
}

func NewPostgresRegistrationTokensTable(db *sql.DB) (tables.RegistrationTokensTable, error) {
	s := &registrationTokenStatements{}
	_, err := db.Exec(registrationTokensSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.selectTokenStatement, selectTokenSQL},
		{&s.insertTokenStatment, insertTokenSQL},
	}.Prepare(db)
}

func (s *registrationTokenStatements) RegistrationTokenExists(ctx context.Context, tx *sql.Tx, token string) (bool, error) {
	var existingToken string
	stmt := s.selectTokenStatement
	err := stmt.QueryRowContext(ctx, token).Scan(&existingToken)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *registrationTokenStatements) InsertRegistrationToken(ctx context.Context, tx *sql.Tx, token string, usesAllowed int32, expiryTime int64) (bool, error) {
	stmt := sqlutil.TxStmt(tx, s.insertTokenStatment)
	pending := 0
	completed := 0
	_, err := stmt.ExecContext(ctx, token, nil, expiryTime, pending, completed)
	if err != nil {
		return false, err
	}
	return true, nil
}
