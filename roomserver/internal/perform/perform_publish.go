// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package perform

import (
	"context"

	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/storage"
)

type Publisher struct {
	DB storage.Database
}

// PerformPublish publishes or unpublishes a room from the room directory. Returns a database error, if any.
func (r *Publisher) PerformPublish(
	ctx context.Context,
	req *api.PerformPublishRequest,
) error {
	return r.DB.PublishRoom(ctx, req.RoomID, req.AppserviceID, req.NetworkID, req.Visibility == "public")
}
