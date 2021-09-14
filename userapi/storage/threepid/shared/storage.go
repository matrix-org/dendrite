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
		return err
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
