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
	"time"
)

const mediaSchema = `
-- The events table holds metadata for each media upload to the local server,
-- the actual file is stored separately.
CREATE TABLE IF NOT EXISTS media_repository (
    -- The id used to refer to the media.
    -- This is a base64-encoded sha256 hash of the file data
    media_id TEXT PRIMARY KEY,
    -- The origin of the media as requested by the client.
    media_origin TEXT NOT NULL,
    -- The MIME-type of the media file.
    content_type TEXT NOT NULL,
    -- The HTTP Content-Disposition header for the media file.
    content_disposition TEXT NOT NULL,
    -- Size of the media file in bytes.
    file_size BIGINT NOT NULL,
    -- When the content was uploaded in ms.
    created_ts BIGINT NOT NULL,
    -- The name with which the media was uploaded.
    upload_name TEXT NOT NULL,
    -- The user who uploaded the file.
    user_id TEXT NOT NULL,
    UNIQUE(media_id, media_origin)
);
`

const insertMediaSQL = `
INSERT INTO media_repository (media_id, media_origin, content_type, content_disposition, file_size, created_ts, upload_name, user_id)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`

const selectMediaSQL = `
SELECT content_type, content_disposition, file_size, upload_name FROM media_repository WHERE media_id = $1 AND media_origin = $2
`

type mediaStatements struct {
	insertMediaStmt *sql.Stmt
	selectMediaStmt *sql.Stmt
}

func (s *mediaStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(mediaSchema)
	if err != nil {
		return
	}

	return statementList{
		{&s.insertMediaStmt, insertMediaSQL},
		{&s.selectMediaStmt, selectMediaSQL},
	}.prepare(db)
}

func (s *mediaStatements) insertMedia(mediaID string, mediaOrigin string, contentType string,
	contentDisposition string, fileSize int64, uploadName string, userID string) error {
	_, err := s.insertMediaStmt.Exec(
		mediaID, mediaOrigin, contentType, contentDisposition, fileSize,
		int64(time.Now().UnixNano()/1000000), uploadName, userID,
	)
	return err
}

func (s *mediaStatements) selectMedia(mediaID string, mediaOrigin string) (string, string, int64, string, error) {
	var contentType string
	var contentDisposition string
	var fileSize int64
	var uploadName string
	err := s.selectMediaStmt.QueryRow(mediaID, mediaOrigin).Scan(&contentType, &contentDisposition, &fileSize, &uploadName)
	return string(contentType), string(contentDisposition), int64(fileSize), string(uploadName), err
}
