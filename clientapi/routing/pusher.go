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
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	pushserver "github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/userapi/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// https://matrix.org/docs/spec/client_server/r0.6.1#get-matrix-client-r0-pushers
type pusherJSON struct {
	PushKey           string                     `json:"pushkey"`
	Kind              string                     `json:"kind"`
	AppID             string                     `json:"app_id"`
	AppDisplayName    string                     `json:"app_display_name"`
	DeviceDisplayName string                     `json:"device_display_name"`
	ProfileTag        string                     `json:"profile_tag"`
	Language          string                     `json:"lang"`
	Data              map[string]json.RawMessage `json:"data"`
}

type pushersJSON struct {
	Pushers []pusherJSON `json:"pushers"`
}

// GetPushersByLocalpart handles /_matrix/client/r0/pushers
func GetPushersByLocalpart(
	req *http.Request, device *api.Device,
	userAPI userapi.UserInternalAPI, pushserverAPI pushserver.PushserverInternalAPI,
) util.JSONResponse {
	var queryRes userapi.QueryPushersResponse
	err := userAPI.QueryPushers(req.Context(), &userapi.QueryPushersRequest{
		UserID: device.UserID,
	}, &queryRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("QueryPushers failed")
		return jsonerror.InternalServerError()
	}

	res := pushersJSON{
		Pushers: []pusherJSON{},
	}

	//data := map[string]json.RawMessage;
	var data map[string]json.RawMessage
	for _, pusher := range queryRes.Pushers {
		json.Unmarshal([]byte(pusher.Data), &data)
		res.Pushers = append(res.Pushers, pusherJSON{
			PushKey:           pusher.PushKey,
			Kind:              pusher.Kind,
			AppID:             pusher.AppID,
			AppDisplayName:    pusher.AppDisplayName,
			DeviceDisplayName: pusher.DeviceDisplayName,
			ProfileTag:        pusher.ProfileTag,
			Language:          pusher.Language,
			Data:              data,
		})
	}

	logrus.Debugf("😁 HTTP returning %d pushers", len(res.Pushers))
	logrus.Debugf("🔮 Pushers %v", res.Pushers)
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}

// SetPushersByLocalpart handles /_matrix/client/r0/pushers/set
// This endpoint allows the creation, modification and deletion of pushers for this user ID.
// The behaviour of this endpoint varies depending on the values in the JSON body.
func SetPusherByLocalpart(
	req *http.Request, device *api.Device,
	userAPI userapi.UserInternalAPI, pushserverAPI pushserver.PushserverInternalAPI,
) util.JSONResponse {
	var deletionRes userapi.PerformPusherDeletionResponse
	body := pusherJSON{}

	if resErr := httputil.UnmarshalJSONRequest(req, &body); resErr != nil {
		return *resErr
	}

	var queryRes userapi.QueryPushersResponse
	err := userAPI.QueryPushers(req.Context(), &userapi.QueryPushersRequest{
		UserID: device.UserID,
	}, &queryRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("QueryPushers failed")
		return jsonerror.InternalServerError()
	}

	var targetPusher *userapi.Pusher
	for _, pusher := range queryRes.Pushers {
		if pusher.PushKey == body.PushKey {
			targetPusher = &pusher
			break
		}
	}

	// No Pusher exists with the given PushKey for current user
	if targetPusher == nil {
		// Create a new Pusher for current user
		var pusherResponse userapi.PerformPusherCreationResponse
		err = userAPI.PerformPusherCreation(req.Context(), &userapi.PerformPusherCreationRequest{
			Device:            device,
			PushKey:           body.PushKey,
			Kind:              body.Kind,
			AppID:             body.AppID,
			AppDisplayName:    body.AppDisplayName,
			DeviceDisplayName: body.DeviceDisplayName,
			ProfileTag:        body.ProfileTag,
			Language:          body.Language,
			Data:              body.Data,
		}, &pusherResponse)

		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("PerformPusherCreation failed")
			return jsonerror.InternalServerError()
		}
	} else if body.Kind == "" {
		if targetPusher == nil {
			return util.JSONResponse{
				Code: http.StatusNotFound,
				JSON: jsonerror.NotFound("Unknown pusher"),
			}
		}

		// if kind is null, delete the pusher! 🗑
		err = userAPI.PerformPusherDeletion(req.Context(), &userapi.PerformPusherDeletionRequest{
			AppID:   targetPusher.AppID,
			PushKey: targetPusher.PushKey,
			UserID:  targetPusher.UserID,
		}, &deletionRes)

		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("PerformPusherDeletion failed")
			return jsonerror.InternalServerError()
		}
	} else {
		var pusherResponse userapi.PerformPusherUpdateResponse
		err = userAPI.PerformPusherUpdate(req.Context(), &userapi.PerformPusherUpdateRequest{
			PushKey:           body.PushKey,
			Kind:              body.Kind,
			AppID:             body.AppID,
			AppDisplayName:    body.AppDisplayName,
			DeviceDisplayName: body.DeviceDisplayName,
			ProfileTag:        body.ProfileTag,
			Language:          body.Language,
			Data:              body.Data,
		}, &pusherResponse)

		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("PerformPusherUpdate failed")
			return jsonerror.InternalServerError()
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
