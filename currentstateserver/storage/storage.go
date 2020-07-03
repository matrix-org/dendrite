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
	"net/url"

	"github.com/matrix-org/dendrite/currentstateserver/storage/postgres"
	"github.com/matrix-org/dendrite/currentstateserver/storage/sqlite3"
	"github.com/matrix-org/dendrite/internal/sqlutil"
)

// NewDatabase opens a database connection.
func NewDatabase(dataSourceName string, dbProperties sqlutil.DbProperties) (Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return postgres.NewDatabase(dataSourceName, dbProperties)
	}
	switch uri.Scheme {
	case "postgres":
		return postgres.NewDatabase(dataSourceName, dbProperties)
	case "file":
		return sqlite3.NewDatabase(dataSourceName)
	default:
		return postgres.NewDatabase(dataSourceName, dbProperties)
	}
}
