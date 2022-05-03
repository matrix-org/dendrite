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

	"github.com/matrix-org/dendrite/federationapi/storage/postgres"
	"github.com/matrix-org/dendrite/federationapi/storage/sqlite3"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

// NewDatabase opens a new database
func NewDatabase(base *base.BaseDendrite, dbProperties *config.DatabaseOptions, cache caching.FederationCache, serverName gomatrixserverlib.ServerName) (Database, error) {
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		return sqlite3.NewDatabase(base, dbProperties, cache, serverName)
	case dbProperties.ConnectionString.IsPostgres():
		return postgres.NewDatabase(base, dbProperties, cache, serverName)
	default:
		return nil, fmt.Errorf("unexpected database type")
	}
}
