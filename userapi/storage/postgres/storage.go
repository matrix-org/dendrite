// Copyright 2017 Vector Creations Ltd
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

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/storage/postgres/deltas"
	"github.com/matrix-org/dendrite/userapi/storage/shared"

	// Import the postgres database driver.
	_ "github.com/lib/pq"
)

// NewDatabase creates a new accounts and profiles database
func NewDatabase(base *base.BaseDendrite, dbProperties *config.DatabaseOptions, serverName gomatrixserverlib.ServerName, bcryptCost int, openIDTokenLifetimeMS int64, loginTokenLifetime time.Duration, serverNoticesLocalpart string) (*shared.Database, error) {
	db, writer, err := base.DatabaseConnection(dbProperties, sqlutil.NewDummyWriter())
	if err != nil {
		return nil, err
	}

	m := sqlutil.NewMigrator(db)
	m.AddMigrations(sqlutil.Migration{
		Version: "userapi: rename tables",
		Up:      deltas.UpRenameTables,
		Down:    deltas.DownRenameTables,
	})
	m.AddMigrations(sqlutil.Migration{
		Version: "userapi: server names",
		Up: func(ctx context.Context, txn *sql.Tx) error {
			return deltas.UpServerNames(ctx, txn, serverName)
		},
	})
	if err = m.Up(base.Context()); err != nil {
		return nil, err
	}

	accountsTable, err := NewPostgresAccountsTable(db, serverName)
	if err != nil {
		return nil, fmt.Errorf("NewPostgresAccountsTable: %w", err)
	}
	accountDataTable, err := NewPostgresAccountDataTable(db)
	if err != nil {
		return nil, fmt.Errorf("NewPostgresAccountDataTable: %w", err)
	}
	devicesTable, err := NewPostgresDevicesTable(db, serverName)
	if err != nil {
		return nil, fmt.Errorf("NewPostgresDevicesTable: %w", err)
	}
	keyBackupTable, err := NewPostgresKeyBackupTable(db)
	if err != nil {
		return nil, fmt.Errorf("NewPostgresKeyBackupTable: %w", err)
	}
	keyBackupVersionTable, err := NewPostgresKeyBackupVersionTable(db)
	if err != nil {
		return nil, fmt.Errorf("NewPostgresKeyBackupVersionTable: %w", err)
	}
	loginTokenTable, err := NewPostgresLoginTokenTable(db)
	if err != nil {
		return nil, fmt.Errorf("NewPostgresLoginTokenTable: %w", err)
	}
	openIDTable, err := NewPostgresOpenIDTable(db, serverName)
	if err != nil {
		return nil, fmt.Errorf("NewPostgresOpenIDTable: %w", err)
	}
	profilesTable, err := NewPostgresProfilesTable(db, serverNoticesLocalpart)
	if err != nil {
		return nil, fmt.Errorf("NewPostgresProfilesTable: %w", err)
	}
	threePIDTable, err := NewPostgresThreePIDTable(db)
	if err != nil {
		return nil, fmt.Errorf("NewPostgresThreePIDTable: %w", err)
	}
	pusherTable, err := NewPostgresPusherTable(db)
	if err != nil {
		return nil, fmt.Errorf("NewPostgresPusherTable: %w", err)
	}
	notificationsTable, err := NewPostgresNotificationTable(db)
	if err != nil {
		return nil, fmt.Errorf("NewPostgresNotificationTable: %w", err)
	}
	statsTable, err := NewPostgresStatsTable(db, serverName)
	if err != nil {
		return nil, fmt.Errorf("NewPostgresStatsTable: %w", err)
	}

	m = sqlutil.NewMigrator(db)
	m.AddMigrations(sqlutil.Migration{
		Version: "userapi: server names populate",
		Up: func(ctx context.Context, txn *sql.Tx) error {
			return deltas.UpServerNamesPopulate(ctx, txn, serverName)
		},
	})
	if err = m.Up(base.Context()); err != nil {
		return nil, err
	}

	return &shared.Database{
		AccountDatas:          accountDataTable,
		Accounts:              accountsTable,
		Devices:               devicesTable,
		KeyBackups:            keyBackupTable,
		KeyBackupVersions:     keyBackupVersionTable,
		LoginTokens:           loginTokenTable,
		OpenIDTokens:          openIDTable,
		Profiles:              profilesTable,
		ThreePIDs:             threePIDTable,
		Pushers:               pusherTable,
		Notifications:         notificationsTable,
		Stats:                 statsTable,
		ServerName:            serverName,
		DB:                    db,
		Writer:                writer,
		LoginTokenLifetime:    loginTokenLifetime,
		BcryptCost:            bcryptCost,
		OpenIDTokenLifetimeMS: openIDTokenLifetimeMS,
	}, nil
}
