// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package tables

import (
	"context"
	"database/sql"

	"github.com/element-hq/dendrite/mediaapi/types"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type Thumbnails interface {
	InsertThumbnail(ctx context.Context, txn *sql.Tx, thumbnailMetadata *types.ThumbnailMetadata) error
	SelectThumbnail(
		ctx context.Context, txn *sql.Tx,
		mediaID types.MediaID, mediaOrigin spec.ServerName,
		width, height int,
		resizeMethod string,
	) (*types.ThumbnailMetadata, error)
	SelectThumbnails(
		ctx context.Context, txn *sql.Tx, mediaID types.MediaID,
		mediaOrigin spec.ServerName,
	) ([]*types.ThumbnailMetadata, error)
}

type MediaRepository interface {
	InsertMedia(ctx context.Context, txn *sql.Tx, mediaMetadata *types.MediaMetadata) error
	SelectMedia(ctx context.Context, txn *sql.Tx, mediaID types.MediaID, mediaOrigin spec.ServerName) (*types.MediaMetadata, error)
	SelectMediaByHash(
		ctx context.Context, txn *sql.Tx,
		mediaHash types.Base64Hash, mediaOrigin spec.ServerName,
	) (*types.MediaMetadata, error)
}
