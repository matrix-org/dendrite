// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"context"

	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	MediaRepository
	Thumbnails
}

type MediaRepository interface {
	StoreMediaMetadata(ctx context.Context, mediaMetadata *types.MediaMetadata) error
	GetMediaMetadata(ctx context.Context, mediaID types.MediaID, mediaOrigin gomatrixserverlib.ServerName) (*types.MediaMetadata, error)
	GetMediaMetadataByHash(ctx context.Context, mediaHash types.Base64Hash, mediaOrigin gomatrixserverlib.ServerName) (*types.MediaMetadata, error)
}

type Thumbnails interface {
	StoreThumbnail(ctx context.Context, thumbnailMetadata *types.ThumbnailMetadata) error
	GetThumbnail(ctx context.Context, mediaID types.MediaID, mediaOrigin gomatrixserverlib.ServerName, width, height int, resizeMethod string) (*types.ThumbnailMetadata, error)
	GetThumbnails(ctx context.Context, mediaID types.MediaID, mediaOrigin gomatrixserverlib.ServerName) ([]*types.ThumbnailMetadata, error)
}
