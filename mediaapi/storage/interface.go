// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package storage

import (
	"context"

	"github.com/element-hq/dendrite/mediaapi/types"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type Database interface {
	MediaRepository
	Thumbnails
}

type MediaRepository interface {
	StoreMediaMetadata(ctx context.Context, mediaMetadata *types.MediaMetadata) error
	GetMediaMetadata(ctx context.Context, mediaID types.MediaID, mediaOrigin spec.ServerName) (*types.MediaMetadata, error)
	GetMediaMetadataByHash(ctx context.Context, mediaHash types.Base64Hash, mediaOrigin spec.ServerName) (*types.MediaMetadata, error)
}

type Thumbnails interface {
	StoreThumbnail(ctx context.Context, thumbnailMetadata *types.ThumbnailMetadata) error
	GetThumbnail(ctx context.Context, mediaID types.MediaID, mediaOrigin spec.ServerName, width, height int, resizeMethod string) (*types.ThumbnailMetadata, error)
	GetThumbnails(ctx context.Context, mediaID types.MediaID, mediaOrigin spec.ServerName) ([]*types.ThumbnailMetadata, error)
}
