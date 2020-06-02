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

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/publicroomsapi/storage/postgres"
	"github.com/matrix-org/dendrite/publicroomsapi/storage/sqlite3"
	"github.com/matrix-org/gomatrixserverlib"
)

const schemePostgres = "postgres"
const schemeFile = "file"

// NewPublicRoomsServerDatabase opens a database connection.
func NewPublicRoomsServerDatabase(dataSourceName string, dbProperties internal.DbProperties, localServerName gomatrixserverlib.ServerName) (Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return postgres.NewPublicRoomsServerDatabase(dataSourceName, dbProperties, localServerName)
	}
	switch uri.Scheme {
	case schemePostgres:
		return postgres.NewPublicRoomsServerDatabase(dataSourceName, dbProperties, localServerName)
	case schemeFile:
		return sqlite3.NewPublicRoomsServerDatabase(dataSourceName, localServerName)
	default:
		return postgres.NewPublicRoomsServerDatabase(dataSourceName, dbProperties, localServerName)
	}
}
