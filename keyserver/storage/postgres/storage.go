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

package postgres

import (
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/keyserver/storage/shared"
)

// NewDatabase creates a new sync server database
func NewDatabase(dbDataSourceName string, dbProperties sqlutil.DbProperties) (*shared.Database, error) {
	var err error
	db, err := sqlutil.Open("postgres", dbDataSourceName, dbProperties)
	if err != nil {
		return nil, err
	}
	otk, err := NewPostgresOneTimeKeysTable(db)
	if err != nil {
		return nil, err
	}
	dk, err := NewPostgresDeviceKeysTable(db)
	if err != nil {
		return nil, err
	}
	return &shared.Database{
		DB:               db,
		OneTimeKeysTable: otk,
		DeviceKeysTable:  dk,
	}, nil
}
