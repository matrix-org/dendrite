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

package sqlite3

import (
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/keyserver/storage/shared"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
)

func NewDatabase(base *base.BaseDendrite, dbProperties *config.DatabaseOptions) (*shared.Database, error) {
	db, writer, err := base.DatabaseConnection(dbProperties, sqlutil.NewExclusiveWriter())
	if err != nil {
		return nil, err
	}
	otk, err := NewSqliteOneTimeKeysTable(db)
	if err != nil {
		return nil, err
	}
	dk, err := NewSqliteDeviceKeysTable(db)
	if err != nil {
		return nil, err
	}
	kc, err := NewSqliteKeyChangesTable(db)
	if err != nil {
		return nil, err
	}
	sdl, err := NewSqliteStaleDeviceListsTable(db)
	if err != nil {
		return nil, err
	}
	csk, err := NewSqliteCrossSigningKeysTable(db)
	if err != nil {
		return nil, err
	}
	css, err := NewSqliteCrossSigningSigsTable(db)
	if err != nil {
		return nil, err
	}

	if err = kc.Prepare(); err != nil {
		return nil, err
	}
	d := &shared.Database{
		DB:                    db,
		Writer:                writer,
		OneTimeKeysTable:      otk,
		DeviceKeysTable:       dk,
		KeyChangesTable:       kc,
		StaleDeviceListsTable: sdl,
		CrossSigningKeysTable: csk,
		CrossSigningSigsTable: css,
	}
	return d, nil
}
