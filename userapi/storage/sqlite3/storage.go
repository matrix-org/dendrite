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

package sqlite3

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"

	"github.com/matrix-org/dendrite/userapi/storage/shared"
	"github.com/matrix-org/dendrite/userapi/storage/sqlite3/deltas"
)

// NewDatabase creates a new accounts and profiles database
func NewDatabase(base *base.BaseDendrite, dbProperties *config.DatabaseOptions, serverName gomatrixserverlib.ServerName, bcryptCost int, openIDTokenLifetimeMS int64, loginTokenLifetime time.Duration, serverNoticesLocalpart string) (*shared.Database, error) {
	db, writer, err := base.DatabaseConnection(dbProperties, sqlutil.NewExclusiveWriter())
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

	accountsTable, err := NewSQLiteAccountsTable(db, serverName)
	if err != nil {
		return nil, fmt.Errorf("NewSQLiteAccountsTable: %w", err)
	}
	accountDataTable, err := NewSQLiteAccountDataTable(db)
	if err != nil {
		return nil, fmt.Errorf("NewSQLiteAccountDataTable: %w", err)
	}
	devicesTable, err := NewSQLiteDevicesTable(db, serverName)
	if err != nil {
		return nil, fmt.Errorf("NewSQLiteDevicesTable: %w", err)
	}
	keyBackupTable, err := NewSQLiteKeyBackupTable(db)
	if err != nil {
		return nil, fmt.Errorf("NewSQLiteKeyBackupTable: %w", err)
	}
	keyBackupVersionTable, err := NewSQLiteKeyBackupVersionTable(db)
	if err != nil {
		return nil, fmt.Errorf("NewSQLiteKeyBackupVersionTable: %w", err)
	}
	loginTokenTable, err := NewSQLiteLoginTokenTable(db)
	if err != nil {
		return nil, fmt.Errorf("NewSQLiteLoginTokenTable: %w", err)
	}
	openIDTable, err := NewSQLiteOpenIDTable(db, serverName)
	if err != nil {
		return nil, fmt.Errorf("NewSQLiteOpenIDTable: %w", err)
	}
	profilesTable, err := NewSQLiteProfilesTable(db, serverNoticesLocalpart)
	if err != nil {
		return nil, fmt.Errorf("NewSQLiteProfilesTable: %w", err)
	}
	threePIDTable, err := NewSQLiteThreePIDTable(db)
	if err != nil {
		return nil, fmt.Errorf("NewSQLiteThreePIDTable: %w", err)
	}
	pusherTable, err := NewSQLitePusherTable(db)
	if err != nil {
		return nil, fmt.Errorf("NewPostgresPusherTable: %w", err)
	}
	notificationsTable, err := NewSQLiteNotificationTable(db)
	if err != nil {
		return nil, fmt.Errorf("NewPostgresNotificationTable: %w", err)
	}
	statsTable, err := NewSQLiteStatsTable(db, serverName)
	if err != nil {
		return nil, fmt.Errorf("NewSQLiteStatsTable: %w", err)
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
