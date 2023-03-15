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

	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/storage/mrd"
	"github.com/matrix-org/dendrite/syncapi/storage/postgres"
	"github.com/matrix-org/dendrite/syncapi/storage/sqlite3"
)

// NewSyncServerDatasource opens a database connection.
func NewSyncServerDatasource(base *base.BaseDendrite, dbProperties *config.DatabaseOptions) (Database, *mrd.Queries, error) {
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		ds, err := sqlite3.NewDatabase(base, dbProperties)
		return ds, nil, err

	case dbProperties.ConnectionString.IsPostgres():
		ds, err := postgres.NewDatabase(base, dbProperties)
		if err != nil {
			return nil, nil, err
		}
		mrq := mrd.New(ds.DB)
		return ds, mrq, nil
	default:
		return nil, nil, fmt.Errorf("unexpected database type")
	}
}
