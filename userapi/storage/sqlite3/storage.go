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
	"fmt"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"

	"github.com/matrix-org/dendrite/userapi/storage/shared"
	"github.com/matrix-org/dendrite/userapi/storage/sqlite3/deltas"

	// Import the postgres database driver.
	_ "github.com/lib/pq"
)

// NewDatabase creates a new accounts and profiles database
func NewDatabase(dbProperties *config.DatabaseOptions, serverName gomatrixserverlib.ServerName, bcryptCost int, openIDTokenLifetimeMS int64, loginTokenLifetime time.Duration, serverNoticesLocalpart string) (*shared.Database, error) {
	db, err := sqlutil.Open(dbProperties)
	if err != nil {
		return nil, err
	}

	m := sqlutil.NewMigrations()
	if _, err = db.Exec(accountsSchema); err != nil {
		// do this so that the migration can and we don't fail on
		// preparing statements for columns that don't exist yet
		return nil, err
	}
	deltas.LoadIsActive(m)
	//deltas.LoadLastSeenTSIP(m)
	deltas.LoadAddAccountType(m)
	if err = m.RunDeltas(db, dbProperties); err != nil {
		return nil, err
	}

	accountDataTable, err := NewSQLiteAccountDataTable(db)
	if err != nil {
		return nil, fmt.Errorf("NewSQLiteAccountDataTable: %w", err)
	}
	accountsTable, err := NewSQLiteAccountsTable(db, serverName)
	if err != nil {
		return nil, fmt.Errorf("NewSQLiteAccountsTable: %w", err)
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
		ServerName:            serverName,
		DB:                    db,
		Writer:                sqlutil.NewExclusiveWriter(),
		LoginTokenLifetime:    loginTokenLifetime,
		BcryptCost:            bcryptCost,
		OpenIDTokenLifetimeMS: openIDTokenLifetimeMS,
	}, nil
}
