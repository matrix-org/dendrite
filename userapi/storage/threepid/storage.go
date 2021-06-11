package threepid

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
)

type Database interface {
	InsertSession(context.Context, *api.Session) error
	GetSession(ctx context.Context, sid string) (*api.Session, error)
	GetSessionByThreePidAndSecret(ctx context.Context, threePid, ClientSecret string) (*api.Session, error)
	UpdateSendAttemptNextLink(ctx context.Context, sid, nextLink string) error
	RemoveSession(ctx context.Context, sid string) error
	ValidateSession(ctx context.Context, sid string, validatedAt int) error
}

type Db struct {
	db           *sql.DB
	writer       sqlutil.Writer
	stm          *sessionStatements
	writeHandler func(func(*sql.Tx) error) error
}

func (d *Db) InsertSession(ctx context.Context, s *api.Session) error {
	h := func(_ *sql.Tx) error {
		_, err := d.stm.insertSessionStmt.ExecContext(ctx, s.Sid, s.ClientSecret, s.ThreePid, s.Token, s.NextLink, s.SendAttempt, s.ValidatedAt, s.Validated)
		return err
	}
	return d.writeHandler(h)
}

func (d *Db) GetSession(ctx context.Context, sid string) (*api.Session, error) {
	s := api.Session{}
	err := d.stm.selectSessionStmt.QueryRowContext(ctx, sid).Scan(&s.ClientSecret, &s.ThreePid, &s.Token, &s.NextLink, &s.Validated, &s.ValidatedAt, &s.SendAttempt)
	s.Sid = sid
	return &s, err
}

func (d *Db) GetSessionByThreePidAndSecret(ctx context.Context, threePid, ClientSecret string) (*api.Session, error) {
	s := api.Session{}
	err := d.stm.selectSessionByThreePidAndCLientSecretStmt.
		QueryRowContext(ctx, threePid, ClientSecret).Scan(
		&s.Sid, &s.Token, &s.NextLink, &s.Validated, &s.ValidatedAt, &s.SendAttempt)
	s.ThreePid = threePid
	s.ClientSecret = ClientSecret
	return &s, err
}

func (d *Db) UpdateSendAttemptNextLink(ctx context.Context, sid, nextLink string) error {
	h := func(_ *sql.Tx) error {
		_, err := d.stm.updateSendAttemptNextLinkStmt.ExecContext(ctx, nextLink, sid)
		return err
	}
	return d.writeHandler(h)
}

func (d *Db) RemoveSession(ctx context.Context, sid string) error {
	h := func(_ *sql.Tx) error {
		_, err := d.stm.deleteSessionStmt.ExecContext(ctx, sid)
		return err
	}
	return d.writeHandler(h)
}

func (d *Db) ValidateSession(ctx context.Context, sid string, validatedAt int) error {
	h := func(_ *sql.Tx) error {
		_, err := d.stm.validateSessionStmt.ExecContext(ctx, true, validatedAt, sid)
		return err
	}
	return d.writeHandler(h)
}

func newSQLiteDatabase(dbProperties *config.DatabaseOptions) (*Db, error) {
	db, err := sqlutil.Open(dbProperties)
	if err != nil {
		return nil, err
	}
	writer := sqlutil.NewExclusiveWriter()
	stmt := sessionStatements{
		db:     db,
		writer: writer,
	}

	// Create tables before executing migrations so we don't fail if the table is missing,
	// and THEN prepare statements so we don't fail due to referencing new columns
	if err = stmt.execSchema(db); err != nil {
		return nil, err
	}
	if err = stmt.prepare(); err != nil {
		return nil, err
	}
	handler := func(f func(tx *sql.Tx) error) error {
		return writer.Do(nil, nil, f)
	}
	return &Db{db, writer, &stmt, handler}, nil
}

func newPostgresDatabase(dbProperties *config.DatabaseOptions) (*Db, error) {
	db, err := sqlutil.Open(dbProperties)
	if err != nil {
		return nil, err
	}
	stmt := sessionStatements{
		db: db,
	}
	// Create tables before executing migrations so we don't fail if the table is missing,
	// and THEN prepare statements so we don't fail due to referencing new columns
	if err = stmt.execSchema(db); err != nil {
		return nil, err
	}
	if err = stmt.prepare(); err != nil {
		return nil, err
	}
	handler := func(f func(tx *sql.Tx) error) error {
		return f(nil)
	}
	return &Db{db, nil, &stmt, handler}, nil
}

func NewDatabase(dbProperties *config.DatabaseOptions) (Database, error) {
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		return newSQLiteDatabase(dbProperties)
	case dbProperties.ConnectionString.IsPostgres():
		return newPostgresDatabase(dbProperties)
	default:
		return nil, fmt.Errorf("unexpected database type")
	}
}
