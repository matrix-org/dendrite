package sqlutil

import (
	"database/sql"
	"fmt"
	"runtime"
	"sort"

	"github.com/matrix-org/dendrite/internal/config"
	"github.com/pressly/goose"
)

type Migrations struct {
	registeredGoMigrations map[int64]*goose.Migration
}

func NewMigrations() *Migrations {
	return &Migrations{
		registeredGoMigrations: make(map[int64]*goose.Migration),
	}
}

// Copy-pasted from goose directly to store migrations into a map we control

// AddMigration adds a migration.
func (m *Migrations) AddMigration(up func(*sql.Tx) error, down func(*sql.Tx) error) {
	_, filename, _, _ := runtime.Caller(1)
	m.AddNamedMigration(filename, up, down)
}

// AddNamedMigration : Add a named migration.
func (m *Migrations) AddNamedMigration(filename string, up func(*sql.Tx) error, down func(*sql.Tx) error) {
	v, _ := goose.NumericComponent(filename)
	migration := &goose.Migration{Version: v, Next: -1, Previous: -1, Registered: true, UpFn: up, DownFn: down, Source: filename}

	if existing, ok := m.registeredGoMigrations[v]; ok {
		panic(fmt.Sprintf("failed to add migration %q: version conflicts with %q", filename, existing.Source))
	}

	m.registeredGoMigrations[v] = migration
}

// RunDeltas up to the latest version.
func (m *Migrations) RunDeltas(db *sql.DB, props *config.DatabaseOptions) error {
	maxVer := goose.MaxVersion
	minVer := int64(0)
	migrations, err := m.collect(minVer, maxVer)
	if err != nil {
		return fmt.Errorf("RunDeltas: Failed to collect migrations: %w", err)
	}
	if props.ConnectionString.IsPostgres() {
		if err = goose.SetDialect("postgres"); err != nil {
			return err
		}
	} else if props.ConnectionString.IsSQLite() {
		if err = goose.SetDialect("sqlite3"); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("Unknown connection string: %s", props.ConnectionString)
	}
	for {
		current, err := goose.EnsureDBVersion(db)
		if err != nil {
			return fmt.Errorf("RunDeltas: Failed to EnsureDBVersion: %w", err)
		}

		next, err := migrations.Next(current)
		if err != nil {
			if err == goose.ErrNoNextVersion {
				return nil
			}

			return fmt.Errorf("RunDeltas: Failed to load next migration to %+v : %w", next, err)
		}

		if err = next.Up(db); err != nil {
			return fmt.Errorf("RunDeltas: Failed run migration: %w", err)
		}
	}
}

func (m *Migrations) collect(current, target int64) (goose.Migrations, error) {
	var migrations goose.Migrations

	// Go migrations registered via goose.AddMigration().
	for _, migration := range m.registeredGoMigrations {
		v, err := goose.NumericComponent(migration.Source)
		if err != nil {
			return nil, err
		}
		if versionFilter(v, current, target) {
			migrations = append(migrations, migration)
		}
	}

	migrations = sortAndConnectMigrations(migrations)

	return migrations, nil
}

func sortAndConnectMigrations(migrations goose.Migrations) goose.Migrations {
	sort.Sort(migrations)

	// now that we're sorted in the appropriate direction,
	// populate next and previous for each migration
	for i, m := range migrations {
		prev := int64(-1)
		if i > 0 {
			prev = migrations[i-1].Version
			migrations[i-1].Next = m.Version
		}
		migrations[i].Previous = prev
	}

	return migrations
}

func versionFilter(v, current, target int64) bool {

	if target > current {
		return v > current && v <= target
	}

	if target < current {
		return v <= current && v > target
	}

	return false
}
