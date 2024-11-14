package sqlite3

import (
	"context"
	"database/sql"
	"time"

	"github.com/element-hq/dendrite/clientapi/api"
	internal "github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/userapi/storage/tables"
	"golang.org/x/exp/constraints"
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

const getTokenSQL = "" +
	"SELECT pending, completed, uses_allowed, expiry_time FROM userapi_registration_tokens WHERE token = $1"

const deleteTokenSQL = "" +
	"DELETE FROM userapi_registration_tokens WHERE token = $1"

const updateTokenUsesAllowedAndExpiryTimeSQL = "" +
	"UPDATE userapi_registration_tokens SET uses_allowed = $2, expiry_time = $3 WHERE token = $1"

const updateTokenUsesAllowedSQL = "" +
	"UPDATE userapi_registration_tokens SET uses_allowed = $2 WHERE token = $1"

const updateTokenExpiryTimeSQL = "" +
	"UPDATE userapi_registration_tokens SET expiry_time = $2 WHERE token = $1"

type registrationTokenStatements struct {
	selectTokenStatement                         *sql.Stmt
	insertTokenStatement                         *sql.Stmt
	listAllTokensStatement                       *sql.Stmt
	listValidTokensStatement                     *sql.Stmt
	listInvalidTokenStatement                    *sql.Stmt
	getTokenStatement                            *sql.Stmt
	deleteTokenStatement                         *sql.Stmt
	updateTokenUsesAllowedAndExpiryTimeStatement *sql.Stmt
	updateTokenUsesAllowedStatement              *sql.Stmt
	updateTokenExpiryTimeStatement               *sql.Stmt
}

func NewSQLiteRegistrationTokensTable(db *sql.DB) (tables.RegistrationTokensTable, error) {
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
		{&s.getTokenStatement, getTokenSQL},
		{&s.deleteTokenStatement, deleteTokenSQL},
		{&s.updateTokenUsesAllowedAndExpiryTimeStatement, updateTokenUsesAllowedAndExpiryTimeSQL},
		{&s.updateTokenUsesAllowedStatement, updateTokenUsesAllowedSQL},
		{&s.updateTokenExpiryTimeStatement, updateTokenExpiryTimeSQL},
	}.Prepare(db)
}

func (s *registrationTokenStatements) RegistrationTokenExists(ctx context.Context, tx *sql.Tx, token string) (bool, error) {
	var existingToken string
	stmt := sqlutil.TxStmt(tx, s.selectTokenStatement)
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
		getInsertValue(registrationToken.UsesAllowed),
		getInsertValue(registrationToken.ExpiryTime),
		*registrationToken.Pending,
		*registrationToken.Completed)
	if err != nil {
		return false, err
	}
	return true, nil
}

func getInsertValue[t constraints.Integer](in *t) any {
	if in == nil {
		return nil
	}
	return *in
}

func (s *registrationTokenStatements) ListRegistrationTokens(ctx context.Context, tx *sql.Tx, returnAll bool, valid bool) ([]api.RegistrationToken, error) {
	var stmt *sql.Stmt
	var tokens []api.RegistrationToken
	var tokenString string
	var pending, completed, usesAllowed *int32
	var expiryTime *int64
	var rows *sql.Rows
	var err error
	if returnAll {
		stmt = sqlutil.TxStmt(tx, s.listAllTokensStatement)
		rows, err = stmt.QueryContext(ctx)
	} else if valid {
		stmt = sqlutil.TxStmt(tx, s.listValidTokensStatement)
		rows, err = stmt.QueryContext(ctx, time.Now().UnixNano()/int64(time.Millisecond))
	} else {
		stmt = sqlutil.TxStmt(tx, s.listInvalidTokenStatement)
		rows, err = stmt.QueryContext(ctx, time.Now().UnixNano()/int64(time.Millisecond))
	}
	if err != nil {
		return tokens, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "ListRegistrationTokens: rows.close() failed")
	for rows.Next() {
		err = rows.Scan(&tokenString, &pending, &completed, &usesAllowed, &expiryTime)
		if err != nil {
			return tokens, err
		}
		tokenString := tokenString
		pending := pending
		completed := completed
		usesAllowed := usesAllowed
		expiryTime := expiryTime

		tokenMap := api.RegistrationToken{
			Token:       &tokenString,
			Pending:     pending,
			Completed:   completed,
			UsesAllowed: usesAllowed,
			ExpiryTime:  expiryTime,
		}
		tokens = append(tokens, tokenMap)
	}
	return tokens, rows.Err()
}

func (s *registrationTokenStatements) GetRegistrationToken(ctx context.Context, tx *sql.Tx, tokenString string) (*api.RegistrationToken, error) {
	stmt := sqlutil.TxStmt(tx, s.getTokenStatement)
	var pending, completed, usesAllowed *int32
	var expiryTime *int64
	err := stmt.QueryRowContext(ctx, tokenString).Scan(&pending, &completed, &usesAllowed, &expiryTime)
	if err != nil {
		return nil, err
	}
	token := api.RegistrationToken{
		Token:       &tokenString,
		Pending:     pending,
		Completed:   completed,
		UsesAllowed: usesAllowed,
		ExpiryTime:  expiryTime,
	}
	return &token, nil
}

func (s *registrationTokenStatements) DeleteRegistrationToken(ctx context.Context, tx *sql.Tx, tokenString string) error {
	stmt := sqlutil.TxStmt(tx, s.deleteTokenStatement)
	_, err := stmt.ExecContext(ctx, tokenString)
	if err != nil {
		return err
	}
	return nil
}

func (s *registrationTokenStatements) UpdateRegistrationToken(ctx context.Context, tx *sql.Tx, tokenString string, newAttributes map[string]interface{}) (*api.RegistrationToken, error) {
	var stmt *sql.Stmt
	usesAllowed, usesAllowedPresent := newAttributes["usesAllowed"]
	expiryTime, expiryTimePresent := newAttributes["expiryTime"]
	if usesAllowedPresent && expiryTimePresent {
		stmt = sqlutil.TxStmt(tx, s.updateTokenUsesAllowedAndExpiryTimeStatement)
		_, err := stmt.ExecContext(ctx, tokenString, usesAllowed, expiryTime)
		if err != nil {
			return nil, err
		}
	} else if usesAllowedPresent {
		stmt = sqlutil.TxStmt(tx, s.updateTokenUsesAllowedStatement)
		_, err := stmt.ExecContext(ctx, tokenString, usesAllowed)
		if err != nil {
			return nil, err
		}
	} else if expiryTimePresent {
		stmt = sqlutil.TxStmt(tx, s.updateTokenExpiryTimeStatement)
		_, err := stmt.ExecContext(ctx, tokenString, expiryTime)
		if err != nil {
			return nil, err
		}
	}
	return s.GetRegistrationToken(ctx, tx, tokenString)
}
