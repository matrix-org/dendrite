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

package storage

import (
	"database/sql"

	// Import the postgres database driver.
	_ "github.com/lib/pq"
)

// A Database is used to store room events and stream offsets.
type Database struct {
	statements statements
	db         *sql.DB
}

// Open a postgres database.
func Open(dataSourceName string) (*Database, error) {
	var d Database
	var err error
	if d.db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
	if err = d.statements.prepare(d.db); err != nil {
		return nil, err
	}
	return &d, nil
}

// CreateMedia inserts the metadata about the uploaded media into the database.
func (d *Database) CreateMedia(mediaID string, mediaOrigin string, contentType string, contentDisposition string, fileSize int64, uploadName string, userID string) error {
	return d.statements.insertMedia(mediaID, mediaOrigin, contentType, contentDisposition, fileSize, uploadName, userID)
}
