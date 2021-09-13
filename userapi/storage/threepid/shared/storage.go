package shared

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
)

type Database struct {
	writer sqlutil.Writer
	stm    *ThreePidSessionStatements
}

func (d *Database) InsertSession(
	ctx context.Context, clientSecret, threepid, token, nextlink string, validatedAt int64, validated bool, sendAttempt int) (int64, error) {
	var sid int64
	return sid, d.writer.Do(nil, nil, func(_ *sql.Tx) error {
		err := d.stm.insertSessionStmt.QueryRowContext(ctx, clientSecret, threepid, token, nextlink, validatedAt, validated, sendAttempt).Scan(&sid)
		// _, err := d.stm.insertSessionStmt.ExecContext(ctx, clientSecret, threepid, token, nextlink, sendAttempt, validatedAt, validated)
		return err
		// if err != nil {
		// 	return err
		// }
		// err = d.stm.selectSidStmt.QueryRowContext(ctx).Scan(&sid)
	})
}

func (d *Database) GetSession(ctx context.Context, sid int64) (*api.Session, error) {
	s := api.Session{}
	err := d.stm.selectSessionStmt.QueryRowContext(ctx, sid).Scan(&s.ClientSecret, &s.ThreePid, &s.Token, &s.NextLink, &s.ValidatedAt, &s.Validated, &s.SendAttempt)
	s.Sid = sid
	return &s, err
}

func (d *Database) GetSessionByThreePidAndSecret(ctx context.Context, threePid, ClientSecret string) (*api.Session, error) {
	s := api.Session{}
	err := d.stm.selectSessionByThreePidAndCLientSecretStmt.
		QueryRowContext(ctx, threePid, ClientSecret).Scan(
		&s.Sid, &s.Token, &s.NextLink, &s.ValidatedAt, &s.Validated, &s.SendAttempt)
	s.ThreePid = threePid
	s.ClientSecret = ClientSecret
	return &s, err
}

func (d *Database) UpdateSendAttemptNextLink(ctx context.Context, sid int64, nextLink string) error {
	return d.writer.Do(nil, nil, func(_ *sql.Tx) error {
		_, err := d.stm.updateSendAttemptNextLinkStmt.ExecContext(ctx, nextLink, sid)
		return err
	})
}

func (d *Database) DeleteSession(ctx context.Context, sid int64) error {
	return d.writer.Do(nil, nil, func(_ *sql.Tx) error {
		_, err := d.stm.deleteSessionStmt.ExecContext(ctx, sid)
		return err
	})
}

func (d *Database) ValidateSession(ctx context.Context, sid int64, validatedAt int64) error {
	return d.writer.Do(nil, nil, func(_ *sql.Tx) error {
		_, err := d.stm.validateSessionStmt.ExecContext(ctx, true, validatedAt, sid)
		return err
	})
}

func NewDatabase(db *sql.DB, writer sqlutil.Writer) (*Database, error) {
	threePidSessionsTable, err := PrepareThreePidSessionsTable(db)
	if err != nil {
		return nil, err
	}
	d := Database{
		writer: writer,
		stm:    threePidSessionsTable,
	}
	return &d, nil
}

// func newSQLiteDatabase(dbProperties *config.DatabaseOptions) (*Db, error) {
// 	db, err := sqlutil.Open(dbProperties)
// 	if err != nil {
// 		return nil, err
// 	}
// 	writer := sqlutil.NewExclusiveWriter()
// 	stmt := sessionStatements{
// 		db:     db,
// 		writer: writer,
// 	}

// 	// Create tables before executing migrations so we don't fail if the table is missing,
// 	// and THEN prepare statements so we don't fail due to referencing new columns
// 	if err = stmt.execSchema(db); err != nil {
// 		return nil, err
// 	}
// 	if err = stmt.prepare(); err != nil {
// 		return nil, err
// 	}
// 	handler := func(f func(tx *sql.Tx) error) error {
// 		return writer.Do(nil, nil, f)
// 	}
// 	return &Db{db, writer, &stmt, handler}, nil
// }

// func newPostgresDatabase(dbProperties *config.DatabaseOptions) (*Db, error) {
// 	db, err := sqlutil.Open(dbProperties)
// 	if err != nil {
// 		return nil, err
// 	}
// 	stmt := sessionStatements{
// 		db: db,
// 	}
// 	// Create tables before executing migrations so we don't fail if the table is missing,
// 	// and THEN prepare statements so we don't fail due to referencing new columns
// 	if err = stmt.execSchema(db); err != nil {
// 		return nil, err
// 	}
// 	if err = stmt.prepare(); err != nil {
// 		return nil, err
// 	}
// 	handler := func(f func(tx *sql.Tx) error) error {
// 		return f(nil)
// 	}
// 	return &Db{db, nil, &stmt, handler}, nil
// }

// func NewDatabase(dbProperties *config.DatabaseOptions) (Database, error) {
// 	switch {
// 	case dbProperties.ConnectionString.IsSQLite():
// 		return newSQLiteDatabase(dbProperties)
// 	case dbProperties.ConnectionString.IsPostgres():
// 		return newPostgresDatabase(dbProperties)
// 	default:
// 		return nil, fmt.Errorf("unexpected database type")
// 	}
// }
