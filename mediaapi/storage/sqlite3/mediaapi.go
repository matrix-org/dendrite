// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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
	// Import the postgres database driver.
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/mediaapi/storage/shared"
	"github.com/matrix-org/dendrite/setup/config"
)

// NewDatabase opens a SQLIte database.
func NewDatabase(conMan *sqlutil.Connections, dbProperties *config.DatabaseOptions) (*shared.Database, error) {
	db, writer, err := conMan.Connection(dbProperties)
	if err != nil {
		return nil, err
	}
	mediaRepo, err := NewSQLiteMediaRepositoryTable(db)
	if err != nil {
		return nil, err
	}
	thumbnails, err := NewSQLiteThumbnailsTable(db)
	if err != nil {
		return nil, err
	}
	return &shared.Database{
		MediaRepository: mediaRepo,
		Thumbnails:      thumbnails,
		DB:              db,
		Writer:          writer,
	}, nil
}
