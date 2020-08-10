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

// +build !wasm

package storage

import (
	"fmt"

	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/keyserver/storage/postgres"
	"github.com/matrix-org/dendrite/keyserver/storage/sqlite3"
)

// NewDatabase opens a new Postgres or Sqlite database (based on dataSourceName scheme)
// and sets postgres connection parameters
func NewDatabase(dbProperties *config.DatabaseOptions) (Database, error) {
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		return sqlite3.NewDatabase(dbProperties)
	case dbProperties.ConnectionString.IsPostgres():
		return postgres.NewDatabase(dbProperties)
	default:
		return nil, fmt.Errorf("unexpected database type")
	}
}
