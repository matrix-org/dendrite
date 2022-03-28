// Copyright 2020 The Matrix.org Foundation C.I.C.
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

//go:build !wasm
// +build !wasm

package storage

import (
	"fmt"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/storage/postgres"
	"github.com/matrix-org/dendrite/userapi/storage/sqlite3"
)

// NewDatabase opens a new Postgres or Sqlite database (based on dataSourceName scheme)
// and sets postgres connection parameters
func NewDatabase(dbProperties *config.DatabaseOptions, serverName gomatrixserverlib.ServerName, bcryptCost int, openIDTokenLifetimeMS int64, loginTokenLifetime time.Duration, serverNoticesLocalpart string) (Database, error) {
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		return sqlite3.NewDatabase(dbProperties, serverName, bcryptCost, openIDTokenLifetimeMS, loginTokenLifetime, serverNoticesLocalpart)
	case dbProperties.ConnectionString.IsPostgres():
		return postgres.NewDatabase(dbProperties, serverName, bcryptCost, openIDTokenLifetimeMS, loginTokenLifetime, serverNoticesLocalpart)
	default:
		return nil, fmt.Errorf("unexpected database type")
	}
}
