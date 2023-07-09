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

package routing

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// https://matrix.org/docs/spec/client_server/r0.6.0#get-matrix-client-r0-rooms-roomid-joined-members
type getLocationSyncResponse struct {
	RecentLocations types.MultiRoom `json:"recent_locations"`
}

// GetLocationSync return each VISIBLE user's most recent location update in the room.
// This is used for the case of newly joined users which require catching up on location events of users,
// but aren't able to retrieve certain location events from /sync, since they joined after them.
func GetLocationSync(
	req *http.Request, device *userapi.Device, roomID string,
	syncDB storage.Database, rsAPI api.SyncRoomserverAPI,
) util.JSONResponse {
	snapshot, err := syncDB.NewDatabaseSnapshot(req.Context())
	if err != nil {
		logrus.WithError(err).Error("Failed to get snapshot for locations sync")
		return jsonerror.InternalServerError()
	}
	id, err := snapshot.SelectMaxMultiRoomDataEventId(context.Background())
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("failed to get max multiroom data event id")
		return jsonerror.InternalServerError()
	}
	mr, err := snapshot.SelectMultiRoomData(req.Context(), &types.Range{From: 0, To: id}, []string{roomID})
	if err != nil {
		if err != sql.ErrNoRows {
			util.GetLogger(req.Context()).WithError(err).Error("failed to select multiroom data for room")
			return jsonerror.InternalServerError()
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: getLocationSyncResponse{RecentLocations: mr},
	}
}
