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
)

// Migration defines a migration to be run.
type Migration struct {
	// Version is a simple name description/name of this migration
	Version string
	// Up defines function to execute
	Up func(ctx context.Context, txn *sql.Tx) error
	// Down defines function to execute (not implemented yet)
	Down func(ctx context.Context, txn *sql.Tx) error
}

// Migrator the structure used by migrations
type Migrator struct {
	db              *sql.DB
	migrations      []Migration
	knownMigrations map[string]bool
	mutex           *sync.Mutex
}

// NewMigrator creates a new DB migrator
func NewMigrator(db *sql.DB) *Migrator {
	return &Migrator{
		db:              db,
		migrations:      []Migration{},
		knownMigrations: make(map[string]bool),
		mutex:           &sync.Mutex{},
	}
}

// AddMigration adds new migrations to the list.
// De-duplicates migrations by their version
func (m *Migrator) AddMigration(migration Migration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if !m.knownMigrations[migration.Version] {
		m.migrations = append(m.migrations, migration)
		m.knownMigrations[migration.Version] = true
	}
}

// AddMigrations is a convenience method to add migrations
func (m *Migrator) AddMigrations(migrations ...Migration) {
	for _, mig := range migrations {
		m.AddMigration(mig)
	}
}

// Up executes all migrations
func (m *Migrator) Up(ctx context.Context) error {
	var err error
	// ensure there is a table for known migrations
	executedMigrations, err := m.ExecutedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("unable to create/get migrations: %w", err)
	}

	txn, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("unable to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = txn.Rollback()
		}
	}()

	for i := range m.migrations {
		migration := m.migrations[i]
		if !executedMigrations[migration.Version] {
			err = migration.Up(ctx, txn)
			if err != nil {
				return fmt.Errorf("unable to execute migration '%s': %w", migration.Version, err)
			}
			_, err = txn.ExecContext(ctx, "INSERT INTO db_migrations (version, time) VALUES ($1, $2)", migration.Version, time.Now().UTC().Format(time.RFC3339))
			if err != nil {
				return fmt.Errorf("unable to insert executed migrations: %w", err)
			}
		}
	}
	if err = txn.Commit(); err != nil {
		return fmt.Errorf("unable to commit transaction: %w", err)
	}
	return nil
}

// ExecutedMigrations returns a map with already executed migrations
func (m *Migrator) ExecutedMigrations(ctx context.Context) (map[string]bool, error) {
	result := make(map[string]bool)
	_, err := m.db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS db_migrations ( version TEXT, time TEXT );")
	if err != nil {
		return nil, fmt.Errorf("unable to create db_migrations: %w", err)
	}
	rows, err := m.db.QueryContext(ctx, "SELECT version FROM db_migrations")
	if err != nil {
		return nil, fmt.Errorf("unable to query db_migrations: %w", err)
	}
	var version string
	for rows.Next() {
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("unable to scan version: %w", err)
		}
		result[version] = true
	}

	return result, rows.Err()
}
