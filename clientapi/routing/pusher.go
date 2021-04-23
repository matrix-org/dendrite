// Copyright 2021 Dan Peleg <dan@globekeeper.com>
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
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/userapi/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

// https://matrix.org/docs/spec/client_server/r0.6.1#get-matrix-client-r0-pushers
type pusherJSON struct {
	PusherID          string         `json:"pusher_id"`
	PushKey           string         `json:"pushkey"`
	Kind              string         `json:"kind"`
	AppID             string         `json:"app_id"`
	AppDisplayName    string         `json:"app_display_name"`
	DeviceDisplayName string         `json:"device_display_name"`
	ProfileTag        string         `json:"profile_tag"`
	Language          string         `json:"lang"`
	Data              pusherDataJSON `json:"data"`
}

type pushersJSON struct {
	Pushers []pusherJSON `json:"pushers"`
}

type pusherDataJSON struct {
	URL    string `json:"url"`
	Format string `json:"format"`
}

// GetPushersByLocalpart handles /pushers
func GetPushersByLocalpart(
	req *http.Request, userAPI userapi.UserInternalAPI, pusher *api.Pusher,
) util.JSONResponse {
	var queryRes userapi.QueryPushersResponse
	err := userAPI.QueryPushers(req.Context(), &userapi.QueryPushersRequest{
		UserID: pusher.UserID,
	}, &queryRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("QueryPushers failed")
		return jsonerror.InternalServerError()
	}

	res := pushersJSON{}

	for _, pusher := range queryRes.Pushers {
		res.Pushers = append(res.Pushers, pusherJSON{
			PusherID:          pusher.ID,
			PushKey:           pusher.PushKey,
			Kind:              pusher.Kind,
			AppID:             pusher.AppID,
			AppDisplayName:    pusher.AppDisplayName,
			DeviceDisplayName: pusher.DeviceDisplayName,
			ProfileTag:        pusher.ProfileTag,
			Language:          pusher.Language,
			Data:              pusherDataJSON(pusher.Data),
		})
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}
