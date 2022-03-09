package sqlutil_test

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	_ "github.com/mattn/go-sqlite3"
)

var dummyMigrations = []sqlutil.Migration{
	{
		Version: "init",
		Up: func(ctx context.Context, txn *sql.Tx) error {
			_, err := txn.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS dummy ( test TEXT );")
			return err
		},
	},
	{
		Version: "v2",
		Up: func(ctx context.Context, txn *sql.Tx) error {
			_, err := txn.ExecContext(ctx, "ALTER TABLE dummy ADD COLUMN test2 TEXT;")
			return err
		},
	},
	{
		Version: "v2", // duplicate, this migration will be skipped
		Up: func(ctx context.Context, txn *sql.Tx) error {
			_, err := txn.ExecContext(ctx, "ALTER TABLE dummy ADD COLUMN test2 TEXT;")
			return err
		},
	},
	{
		Version: "multiple execs",
		Up: func(ctx context.Context, txn *sql.Tx) error {
			_, err := txn.ExecContext(ctx, "ALTER TABLE dummy ADD COLUMN test3 TEXT;")
			if err != nil {
				return err
			}
			_, err = txn.ExecContext(ctx, "ALTER TABLE dummy ADD COLUMN test4 TEXT;")
			return err
		},
	},
}

var failMigration = sqlutil.Migration{
	Version: "iFail",
	Up: func(ctx context.Context, txn *sql.Tx) error {
		return fmt.Errorf("iFail")
	},
	Down: nil,
}

func Test_migrations_Up(t *testing.T) {
	withFail := make([]sqlutil.Migration, len(dummyMigrations))
	copy(withFail, dummyMigrations)
	withFail = append(withFail, failMigration)

	tests := []struct {
		name             string
		connectionString string
		ctx              context.Context
		migrations       []sqlutil.Migration
		wantResult       map[string]bool
		wantErr          bool
	}{
		{
			name:             "dummy migration",
			connectionString: "file::memory:",
			migrations:       dummyMigrations,
			ctx:              context.Background(),
			wantResult: map[string]bool{
				"init":           true,
				"v2":             true,
				"multiple execs": true,
			},
		},
		{
			name:             "with fail",
			connectionString: "file::memory:",
			migrations:       withFail,
			ctx:              context.Background(),
			wantErr:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := sql.Open("sqlite3", tt.connectionString)
			if err != nil {
				t.Errorf("unable to open database: %w", err)
			}
			m := sqlutil.NewMigrator(db)
			m.AddMigrations(tt.migrations...)
			if err := m.Up(tt.ctx); (err != nil) != tt.wantErr {
				t.Errorf("Up() error = %v, wantErr %v", err, tt.wantErr)
			}
			result, err := m.ExecutedMigrations(tt.ctx)
			if err != nil {
				t.Errorf("unable to get executed migrations: %w", err)
			}
			if !tt.wantErr && !reflect.DeepEqual(result, tt.wantResult) {
				t.Errorf("expected: %+v, got %v", tt.wantResult, result)
			}
		})
	}
}
