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
	"github.com/matrix-org/dendrite/mediaapi/types"
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

// StoreMediaMetadata inserts the metadata about the uploaded media into the database.
func (d *Database) StoreMediaMetadata(mediaMetadata *types.MediaMetadata) error {
	return d.statements.insertMedia(mediaMetadata)
}

// GetMediaMetadata possibly selects the metadata about previously uploaded media from the database.
func (d *Database) GetMediaMetadata(mediaID types.MediaID, mediaOrigin types.ServerName, mediaMetadata *types.MediaMetadata) error {
	metadata, err := d.statements.selectMedia(mediaID, mediaOrigin)
	mediaMetadata.ContentType = metadata.ContentType
	mediaMetadata.ContentDisposition = metadata.ContentDisposition
	mediaMetadata.ContentLength = metadata.ContentLength
	mediaMetadata.CreationTimestamp = metadata.CreationTimestamp
	mediaMetadata.UploadName = metadata.UploadName
	mediaMetadata.UserID = metadata.UserID
	return err
}
