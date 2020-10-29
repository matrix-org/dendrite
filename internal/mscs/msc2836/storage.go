package msc2836

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	// StoreRelation stores the parent->child and child->parent relationship for later querying.
	StoreRelation(ctx context.Context, ev *gomatrixserverlib.HeaderedEvent) error
}

type Postgres struct {
	db *sql.DB
}

func NewPostgresDatabase(dbOpts *config.DatabaseOptions) (Database, error) {
	var p Postgres
	var err error
	if p.db, err = sqlutil.Open(dbOpts); err != nil {
		return nil, err
	}
	return &p, nil
}

func (db *Postgres) StoreRelation(ctx context.Context, ev *gomatrixserverlib.HeaderedEvent) error {
	return nil
}

type SQLite struct {
	db     *sql.DB
	writer sqlutil.Writer
}

func NewSQLiteDatabase(dbOpts *config.DatabaseOptions) (Database, error) {
	var s SQLite
	var err error
	if s.db, err = sqlutil.Open(dbOpts); err != nil {
		return nil, err
	}
	s.writer = sqlutil.NewExclusiveWriter()
	return &s, nil
}

func (db *SQLite) StoreRelation(ctx context.Context, ev *gomatrixserverlib.HeaderedEvent) error {
	return nil
}

// NewDatabase loads the database for msc2836
func NewDatabase(dbOpts *config.DatabaseOptions) (Database, error) {
	if dbOpts.ConnectionString.IsPostgres() {
		return NewPostgresDatabase(dbOpts)
	}
	return NewSQLiteDatabase(dbOpts)
}
