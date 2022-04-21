// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package tables

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type Thumbnails interface {
	InsertThumbnail(ctx context.Context, txn *sql.Tx, thumbnailMetadata *types.ThumbnailMetadata) error
	SelectThumbnail(
		ctx context.Context, txn *sql.Tx,
		mediaID types.MediaID, mediaOrigin gomatrixserverlib.ServerName,
		width, height int,
		resizeMethod string,
	) (*types.ThumbnailMetadata, error)
	SelectThumbnails(
		ctx context.Context, txn *sql.Tx, mediaID types.MediaID,
		mediaOrigin gomatrixserverlib.ServerName,
	) ([]*types.ThumbnailMetadata, error)
}

type MediaRepository interface {
	InsertMedia(ctx context.Context, txn *sql.Tx, mediaMetadata *types.MediaMetadata) error
	SelectMedia(ctx context.Context, txn *sql.Tx, mediaID types.MediaID, mediaOrigin gomatrixserverlib.ServerName) (*types.MediaMetadata, error)
	SelectMediaByHash(
		ctx context.Context, txn *sql.Tx,
		mediaHash types.Base64Hash, mediaOrigin gomatrixserverlib.ServerName,
	) (*types.MediaMetadata, error)
}
