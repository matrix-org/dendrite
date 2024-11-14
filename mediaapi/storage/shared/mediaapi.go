// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package shared

import (
	"context"
	"database/sql"
	"errors"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/mediaapi/storage/tables"
	"github.com/element-hq/dendrite/mediaapi/types"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type Database struct {
	DB              *sql.DB
	Writer          sqlutil.Writer
	MediaRepository tables.MediaRepository
	Thumbnails      tables.Thumbnails
}

// StoreMediaMetadata inserts the metadata about the uploaded media into the database.
// Returns an error if the combination of MediaID and Origin are not unique in the table.
func (d *Database) StoreMediaMetadata(ctx context.Context, mediaMetadata *types.MediaMetadata) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.MediaRepository.InsertMedia(ctx, txn, mediaMetadata)
	})
}

// GetMediaMetadata returns metadata about media stored on this server.
// The media could have been uploaded to this server or fetched from another server and cached here.
// Returns nil metadata if there is no metadata associated with this media.
func (d *Database) GetMediaMetadata(ctx context.Context, mediaID types.MediaID, mediaOrigin spec.ServerName) (*types.MediaMetadata, error) {
	mediaMetadata, err := d.MediaRepository.SelectMedia(ctx, nil, mediaID, mediaOrigin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return mediaMetadata, err
}

// GetMediaMetadataByHash returns metadata about media stored on this server.
// The media could have been uploaded to this server or fetched from another server and cached here.
// Returns nil metadata if there is no metadata associated with this media.
func (d *Database) GetMediaMetadataByHash(ctx context.Context, mediaHash types.Base64Hash, mediaOrigin spec.ServerName) (*types.MediaMetadata, error) {
	mediaMetadata, err := d.MediaRepository.SelectMediaByHash(ctx, nil, mediaHash, mediaOrigin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return mediaMetadata, err
}

// StoreThumbnail inserts the metadata about the thumbnail into the database.
// Returns an error if the combination of MediaID and Origin are not unique in the table.
func (d *Database) StoreThumbnail(ctx context.Context, thumbnailMetadata *types.ThumbnailMetadata) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.Thumbnails.InsertThumbnail(ctx, txn, thumbnailMetadata)
	})
}

// GetThumbnail returns metadata about a specific thumbnail.
// The media could have been uploaded to this server or fetched from another server and cached here.
// Returns nil metadata if there is no metadata associated with this thumbnail.
func (d *Database) GetThumbnail(ctx context.Context, mediaID types.MediaID, mediaOrigin spec.ServerName, width, height int, resizeMethod string) (*types.ThumbnailMetadata, error) {
	metadata, err := d.Thumbnails.SelectThumbnail(ctx, nil, mediaID, mediaOrigin, width, height, resizeMethod)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return metadata, err
}

// GetThumbnails returns metadata about all thumbnails for a specific media stored on this server.
// The media could have been uploaded to this server or fetched from another server and cached here.
// Returns nil metadata if there are no thumbnails associated with this media.
func (d *Database) GetThumbnails(ctx context.Context, mediaID types.MediaID, mediaOrigin spec.ServerName) ([]*types.ThumbnailMetadata, error) {
	metadatas, err := d.Thumbnails.SelectThumbnails(ctx, nil, mediaID, mediaOrigin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return metadatas, err
}
