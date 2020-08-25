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

package postgres

import (
	"context"
	"database/sql"

	// Import the postgres database driver.
	_ "github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// Database is used to store metadata about a repository of media files.
type Database struct {
	statements statements
	db         *sql.DB
}

// Open opens a postgres database.
func Open(dbProperties *config.DatabaseOptions) (*Database, error) {
	var d Database
	var err error
	if d.db, err = sqlutil.Open(dbProperties); err != nil {
		return nil, err
	}
	if err = d.statements.prepare(d.db); err != nil {
		return nil, err
	}
	return &d, nil
}

// StoreMediaMetadata inserts the metadata about the uploaded media into the database.
// Returns an error if the combination of MediaID and Origin are not unique in the table.
func (d *Database) StoreMediaMetadata(
	ctx context.Context, mediaMetadata *types.MediaMetadata,
) error {
	return d.statements.media.insertMedia(ctx, mediaMetadata)
}

// GetMediaMetadata returns metadata about media stored on this server.
// The media could have been uploaded to this server or fetched from another server and cached here.
// Returns nil metadata if there is no metadata associated with this media.
func (d *Database) GetMediaMetadata(
	ctx context.Context, mediaID types.MediaID, mediaOrigin gomatrixserverlib.ServerName,
) (*types.MediaMetadata, error) {
	mediaMetadata, err := d.statements.media.selectMedia(ctx, mediaID, mediaOrigin)
	if err != nil && err == sql.ErrNoRows {
		return nil, nil
	}
	return mediaMetadata, err
}

// GetMediaMetadataByHash returns metadata about media stored on this server.
// The media could have been uploaded to this server or fetched from another server and cached here.
// Returns nil metadata if there is no metadata associated with this media.
func (d *Database) GetMediaMetadataByHash(
	ctx context.Context, mediaHash types.Base64Hash, mediaOrigin gomatrixserverlib.ServerName,
) (*types.MediaMetadata, error) {
	mediaMetadata, err := d.statements.media.selectMediaByHash(ctx, mediaHash, mediaOrigin)
	if err != nil && err == sql.ErrNoRows {
		return nil, nil
	}
	return mediaMetadata, err
}

// StoreThumbnail inserts the metadata about the thumbnail into the database.
// Returns an error if the combination of MediaID and Origin are not unique in the table.
func (d *Database) StoreThumbnail(
	ctx context.Context, thumbnailMetadata *types.ThumbnailMetadata,
) error {
	return d.statements.thumbnail.insertThumbnail(ctx, thumbnailMetadata)
}

// GetThumbnail returns metadata about a specific thumbnail.
// The media could have been uploaded to this server or fetched from another server and cached here.
// Returns nil metadata if there is no metadata associated with this thumbnail.
func (d *Database) GetThumbnail(
	ctx context.Context,
	mediaID types.MediaID,
	mediaOrigin gomatrixserverlib.ServerName,
	width, height int,
	resizeMethod string,
) (*types.ThumbnailMetadata, error) {
	thumbnailMetadata, err := d.statements.thumbnail.selectThumbnail(
		ctx, mediaID, mediaOrigin, width, height, resizeMethod,
	)
	if err != nil && err == sql.ErrNoRows {
		return nil, nil
	}
	return thumbnailMetadata, err
}

// GetThumbnails returns metadata about all thumbnails for a specific media stored on this server.
// The media could have been uploaded to this server or fetched from another server and cached here.
// Returns nil metadata if there are no thumbnails associated with this media.
func (d *Database) GetThumbnails(
	ctx context.Context, mediaID types.MediaID, mediaOrigin gomatrixserverlib.ServerName,
) ([]*types.ThumbnailMetadata, error) {
	thumbnails, err := d.statements.thumbnail.selectThumbnails(ctx, mediaID, mediaOrigin)
	if err != nil && err == sql.ErrNoRows {
		return nil, nil
	}
	return thumbnails, err
}
