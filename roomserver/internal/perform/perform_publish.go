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

package perform

import (
	"context"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage"
)

type Publisher struct {
	DB storage.Database
}

func (r *Publisher) PerformPublish(
	ctx context.Context,
	req *api.PerformPublishRequest,
	res *api.PerformPublishResponse,
) error {
	err := r.DB.PublishRoom(ctx, req.RoomID, req.AppserviceID, req.NetworkID, req.Visibility == "public")
	if err != nil {
		res.Error = &api.PerformError{
			Msg: err.Error(),
		}
	}
	return nil
}
