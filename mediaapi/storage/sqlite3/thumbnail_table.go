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
	"context"
	"database/sql"
	"time"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

const thumbnailSchema = `
-- The mediaapi_thumbnail table holds metadata for each thumbnail file stored and accessible to the local server,
-- the actual file is stored separately.
CREATE TABLE IF NOT EXISTS mediaapi_thumbnail (
    media_id TEXT NOT NULL,
    media_origin TEXT NOT NULL,
    content_type TEXT NOT NULL,
    file_size_bytes INTEGER NOT NULL,
    creation_ts INTEGER NOT NULL,
    width INTEGER NOT NULL,
    height INTEGER NOT NULL,
    resize_method TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS mediaapi_thumbnail_index ON mediaapi_thumbnail (media_id, media_origin, width, height, resize_method);
`

const insertThumbnailSQL = `
INSERT INTO mediaapi_thumbnail (media_id, media_origin, content_type, file_size_bytes, creation_ts, width, height, resize_method)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`

// Note: this selects one specific thumbnail
const selectThumbnailSQL = `
SELECT content_type, file_size_bytes, creation_ts FROM mediaapi_thumbnail WHERE media_id = $1 AND media_origin = $2 AND width = $3 AND height = $4 AND resize_method = $5
`

// Note: this selects all thumbnails for a media_origin and media_id
const selectThumbnailsSQL = `
SELECT content_type, file_size_bytes, creation_ts, width, height, resize_method FROM mediaapi_thumbnail WHERE media_id = $1 AND media_origin = $2
`

type thumbnailStatements struct {
	db                   *sql.DB
	writer               sqlutil.Writer
	insertThumbnailStmt  *sql.Stmt
	selectThumbnailStmt  *sql.Stmt
	selectThumbnailsStmt *sql.Stmt
}

func (s *thumbnailStatements) prepare(db *sql.DB, writer sqlutil.Writer) (err error) {
	_, err = db.Exec(thumbnailSchema)
	if err != nil {
		return
	}
	s.db = db
	s.writer = writer

	return statementList{
		{&s.insertThumbnailStmt, insertThumbnailSQL},
		{&s.selectThumbnailStmt, selectThumbnailSQL},
		{&s.selectThumbnailsStmt, selectThumbnailsSQL},
	}.prepare(db)
}

func (s *thumbnailStatements) insertThumbnail(
	ctx context.Context, thumbnailMetadata *types.ThumbnailMetadata,
) error {
	thumbnailMetadata.MediaMetadata.CreationTimestamp = types.UnixMs(time.Now().UnixNano() / 1000000)
	return s.writer.Do(s.db, nil, func(txn *sql.Tx) error {
		stmt := sqlutil.TxStmt(txn, s.insertThumbnailStmt)
		_, err := stmt.ExecContext(
			ctx,
			thumbnailMetadata.MediaMetadata.MediaID,
			thumbnailMetadata.MediaMetadata.Origin,
			thumbnailMetadata.MediaMetadata.ContentType,
			thumbnailMetadata.MediaMetadata.FileSizeBytes,
			thumbnailMetadata.MediaMetadata.CreationTimestamp,
			thumbnailMetadata.ThumbnailSize.Width,
			thumbnailMetadata.ThumbnailSize.Height,
			thumbnailMetadata.ThumbnailSize.ResizeMethod,
		)
		return err
	})
}

func (s *thumbnailStatements) selectThumbnail(
	ctx context.Context,
	mediaID types.MediaID,
	mediaOrigin gomatrixserverlib.ServerName,
	width, height int,
	resizeMethod string,
) (*types.ThumbnailMetadata, error) {
	thumbnailMetadata := types.ThumbnailMetadata{
		MediaMetadata: &types.MediaMetadata{
			MediaID: mediaID,
			Origin:  mediaOrigin,
		},
		ThumbnailSize: types.ThumbnailSize{
			Width:        width,
			Height:       height,
			ResizeMethod: resizeMethod,
		},
	}
	err := s.selectThumbnailStmt.QueryRowContext(
		ctx,
		thumbnailMetadata.MediaMetadata.MediaID,
		thumbnailMetadata.MediaMetadata.Origin,
		thumbnailMetadata.ThumbnailSize.Width,
		thumbnailMetadata.ThumbnailSize.Height,
		thumbnailMetadata.ThumbnailSize.ResizeMethod,
	).Scan(
		&thumbnailMetadata.MediaMetadata.ContentType,
		&thumbnailMetadata.MediaMetadata.FileSizeBytes,
		&thumbnailMetadata.MediaMetadata.CreationTimestamp,
	)
	return &thumbnailMetadata, err
}

func (s *thumbnailStatements) selectThumbnails(
	ctx context.Context, mediaID types.MediaID, mediaOrigin gomatrixserverlib.ServerName,
) ([]*types.ThumbnailMetadata, error) {
	rows, err := s.selectThumbnailsStmt.QueryContext(
		ctx, mediaID, mediaOrigin,
	)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectThumbnails: rows.close() failed")

	var thumbnails []*types.ThumbnailMetadata
	for rows.Next() {
		thumbnailMetadata := types.ThumbnailMetadata{
			MediaMetadata: &types.MediaMetadata{
				MediaID: mediaID,
				Origin:  mediaOrigin,
			},
		}
		err = rows.Scan(
			&thumbnailMetadata.MediaMetadata.ContentType,
			&thumbnailMetadata.MediaMetadata.FileSizeBytes,
			&thumbnailMetadata.MediaMetadata.CreationTimestamp,
			&thumbnailMetadata.ThumbnailSize.Width,
			&thumbnailMetadata.ThumbnailSize.Height,
			&thumbnailMetadata.ThumbnailSize.ResizeMethod,
		)
		if err != nil {
			return nil, err
		}
		thumbnails = append(thumbnails, &thumbnailMetadata)
	}

	return thumbnails, rows.Err()
}
