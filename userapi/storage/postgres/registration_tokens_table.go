package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/matrix-org/dendrite/clientapi/api"
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

const listAllTokensSQL = "" +
	"SELECT * FROM userapi_registration_tokens"

const listValidTokensSQL = "" +
	"SELECT * FROM userapi_registration_tokens WHERE" +
	"(uses_allowed > pending + completed OR uses_allowed IS NULL) AND" +
	"(expiry_time > $1 OR expiry_time IS NULL)"

const listInvalidTokensSQL = "" +
	"SELECT * FROM userapi_registration_tokens WHERE" +
	"(uses_allowed <= pending + completed OR expiry_time <= $1)"

type registrationTokenStatements struct {
	selectTokenStatement      *sql.Stmt
	insertTokenStatement      *sql.Stmt
	listAllTokensStatement    *sql.Stmt
	listValidTokensStatement  *sql.Stmt
	listInvalidTokenStatement *sql.Stmt
}

func NewPostgresRegistrationTokensTable(db *sql.DB) (tables.RegistrationTokensTable, error) {
	s := &registrationTokenStatements{}
	_, err := db.Exec(registrationTokensSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.selectTokenStatement, selectTokenSQL},
		{&s.insertTokenStatement, insertTokenSQL},
		{&s.listAllTokensStatement, listAllTokensSQL},
		{&s.listValidTokensStatement, listValidTokensSQL},
		{&s.listInvalidTokenStatement, listInvalidTokensSQL},
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

func (s *registrationTokenStatements) InsertRegistrationToken(ctx context.Context, tx *sql.Tx, registrationToken *api.RegistrationToken) (bool, error) {
	stmt := sqlutil.TxStmt(tx, s.insertTokenStatement)
	_, err := stmt.ExecContext(
		ctx,
		*registrationToken.Token,
		nullIfZeroInt32(*registrationToken.UsesAllowed),
		nullIfZero(*registrationToken.ExpiryTime),
		*registrationToken.Pending,
		*registrationToken.Completed)
	if err != nil {
		return false, err
	}
	return true, nil
}

func nullIfZero(value int64) interface{} {
	if value == 0 {
		return nil
	}
	return value
}

func nullIfZeroInt32(value int32) interface{} {
	if value == 0 {
		return nil
	}
	return value
}

func (s *registrationTokenStatements) ListRegistrationTokens(ctx context.Context, tx *sql.Tx, returnAll bool, valid bool) ([]api.RegistrationToken, error) {
	var stmt *sql.Stmt
	var tokens []api.RegistrationToken
	var tokenString sql.NullString
	var pending, completed, usesAllowed sql.NullInt32
	var expiryTime sql.NullInt64
	var rows *sql.Rows
	var err error
	if returnAll {
		stmt = s.listAllTokensStatement
		rows, err = stmt.QueryContext(ctx)
	} else if valid {
		stmt = s.listValidTokensStatement
		rows, err = stmt.QueryContext(ctx, time.Now().UnixNano()/int64(time.Millisecond))
	} else {
		stmt = s.listInvalidTokenStatement
		rows, err = stmt.QueryContext(ctx, time.Now().UnixNano()/int64(time.Millisecond))
	}
	if err != nil {
		return tokens, err
	}
	for rows.Next() {
		err = rows.Scan(&tokenString, &pending, &completed, &usesAllowed, &expiryTime)
		if err != nil {
			return tokens, err
		}
		tokenMap := api.RegistrationToken{
			Token:       &tokenString.String,
			Pending:     &pending.Int32,
			Completed:   &pending.Int32,
			UsesAllowed: getReturnValueForInt32(usesAllowed),
			ExpiryTime:  getReturnValueForInt64(expiryTime),
		}
		tokens = append(tokens, tokenMap)
	}
	return tokens, nil
}

func getReturnValueForInt32(value sql.NullInt32) *int32 {
	if value.Valid {
		returnValue := value.Int32
		return &returnValue
	}
	return nil
}

func getReturnValueForInt64(value sql.NullInt64) *int64 {
	if value.Valid {
		returnValue := value.Int64
		return &returnValue
	}
	return nil
}
