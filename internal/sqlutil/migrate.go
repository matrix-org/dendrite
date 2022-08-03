// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlutil

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/internal"
	"github.com/sirupsen/logrus"
)

const createDBMigrationsSQL = "" +
	"CREATE TABLE IF NOT EXISTS db_migrations (" +
	" version TEXT PRIMARY KEY NOT NULL," +
	" time TEXT NOT NULL," +
	" dendrite_version TEXT NOT NULL" +
	");"

const insertVersionSQL = "" +
	"INSERT INTO db_migrations (version, time, dendrite_version)" +
	" VALUES ($1, $2, $3)"

const selectDBMigrationsSQL = "SELECT version FROM db_migrations"

// Migration defines a migration to be run.
type Migration struct {
	// Version is a simple description/name of this migration.
	Version string
	// Up defines the function to execute for an upgrade.
	Up func(ctx context.Context, txn *sql.Tx) error
	// Down defines the function to execute for a downgrade (not implemented yet).
	Down func(ctx context.Context, txn *sql.Tx) error
}

// Migrator
type Migrator struct {
	db              *sql.DB
	migrations      []Migration
	knownMigrations map[string]struct{}
	mutex           *sync.Mutex
}

// NewMigrator creates a new DB migrator.
func NewMigrator(db *sql.DB) *Migrator {
	return &Migrator{
		db:              db,
		migrations:      []Migration{},
		knownMigrations: make(map[string]struct{}),
		mutex:           &sync.Mutex{},
	}
}

// AddMigrations appends migrations to the list of migrations. Migrations are executed
// in the order they are added to the list. De-duplicates migrations using their Version field.
func (m *Migrator) AddMigrations(migrations ...Migration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, mig := range migrations {
		if _, ok := m.knownMigrations[mig.Version]; !ok {
			m.migrations = append(m.migrations, mig)
			m.knownMigrations[mig.Version] = struct{}{}
		}
	}
}

// Up executes all migrations in order they were added.
func (m *Migrator) Up(ctx context.Context) error {
	var (
		err             error
		dendriteVersion = internal.VersionString()
	)
	// ensure there is a table for known migrations
	executedMigrations, err := m.ExecutedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("unable to create/get migrations: %w", err)
	}

	return WithTransaction(m.db, func(txn *sql.Tx) error {
		for i := range m.migrations {
			now := time.Now().UTC().Format(time.RFC3339)
			migration := m.migrations[i]
			logrus.Debugf("Executing database migration '%s'", migration.Version)
			// Skip migration if it was already executed
			if _, ok := executedMigrations[migration.Version]; ok {
				continue
			}
			err = migration.Up(ctx, txn)
			if err != nil {
				return fmt.Errorf("unable to execute migration '%s': %w", migration.Version, err)
			}
			_, err = txn.ExecContext(ctx, insertVersionSQL,
				migration.Version,
				now,
				dendriteVersion,
			)
			if err != nil {
				return fmt.Errorf("unable to insert executed migrations: %w", err)
			}
		}
		return nil
	})
}

// ExecutedMigrations returns a map with already executed migrations in addition to creating the
// migrations table, if it doesn't exist.
func (m *Migrator) ExecutedMigrations(ctx context.Context) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	_, err := m.db.ExecContext(ctx, createDBMigrationsSQL)
	if err != nil {
		return nil, fmt.Errorf("unable to create db_migrations: %w", err)
	}
	rows, err := m.db.QueryContext(ctx, selectDBMigrationsSQL)
	if err != nil {
		return nil, fmt.Errorf("unable to query db_migrations: %w", err)
	}
	defer internal.CloseAndLogIfError(ctx, rows, "ExecutedMigrations: rows.close() failed")
	var version string
	for rows.Next() {
		if err = rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("unable to scan version: %w", err)
		}
		result[version] = struct{}{}
	}

	return result, rows.Err()
}
