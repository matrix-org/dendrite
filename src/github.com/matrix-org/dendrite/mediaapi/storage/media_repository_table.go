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

	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

const mediaSchema = `
-- The media_repository table holds metadata for each media file stored and accessible to the local server,
-- the actual file is stored separately.
CREATE TABLE IF NOT EXISTS media_repository (
    -- The id used to refer to the media.
    -- For uploads to this server this is a base64-encoded sha256 hash of the file data
    -- For media from remote servers, this can be any unique identifier string
    media_id TEXT NOT NULL,
    -- The origin of the media as requested by the client. Should be a homeserver domain.
    media_origin TEXT NOT NULL,
    -- The MIME-type of the media file as specified when uploading.
    content_type TEXT NOT NULL,
    -- Size of the media file in bytes.
    file_size_bytes BIGINT NOT NULL,
    -- When the content was uploaded in UNIX epoch ms.
    creation_ts BIGINT NOT NULL,
    -- The file name with which the media was uploaded.
    upload_name TEXT NOT NULL,
    -- A golang base64 URLEncoding string representation of a SHA-256 hash sum of the file data.
    base64hash TEXT NOT NULL,
    -- The user who uploaded the file. Should be a Matrix user ID.
    user_id TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS media_repository_index ON media_repository (media_id, media_origin);
`

const insertMediaSQL = `
INSERT INTO media_repository (media_id, media_origin, content_type, file_size_bytes, creation_ts, upload_name, base64hash, user_id)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`

const selectMediaSQL = `
SELECT content_type, file_size_bytes, creation_ts, upload_name, base64hash, user_id FROM media_repository WHERE media_id = $1 AND media_origin = $2
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

func (s *mediaStatements) insertMedia(mediaMetadata *types.MediaMetadata) error {
	mediaMetadata.CreationTimestamp = types.UnixMs(time.Now().UnixNano() / 1000000)
	_, err := s.insertMediaStmt.Exec(
		mediaMetadata.MediaID,
		mediaMetadata.Origin,
		mediaMetadata.ContentType,
		mediaMetadata.FileSizeBytes,
		mediaMetadata.CreationTimestamp,
		mediaMetadata.UploadName,
		mediaMetadata.Base64Hash,
		mediaMetadata.UserID,
	)
	return err
}

func (s *mediaStatements) selectMedia(mediaID types.MediaID, mediaOrigin gomatrixserverlib.ServerName) (*types.MediaMetadata, error) {
	mediaMetadata := types.MediaMetadata{
		MediaID: mediaID,
		Origin:  mediaOrigin,
	}
	err := s.selectMediaStmt.QueryRow(
		mediaMetadata.MediaID, mediaMetadata.Origin,
	).Scan(
		&mediaMetadata.ContentType,
		&mediaMetadata.FileSizeBytes,
		&mediaMetadata.CreationTimestamp,
		&mediaMetadata.UploadName,
		&mediaMetadata.Base64Hash,
		&mediaMetadata.UserID,
	)
	return &mediaMetadata, err
}
