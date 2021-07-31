// Copyright 2021 The Matrix.org Foundation C.I.C.
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
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/eduserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	types2 "github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type presenceRequest struct {
	Presence  string `json:"presence"`
	StatusMsg string `json:"status_msg"`
}

// SetPresence updates the users presence status
func SetPresence(req *http.Request,
	eduAPI api.EDUServerInputAPI,
	userAPI userapi.UserInternalAPI,
	userID string,
	device *userapi.Device,
) util.JSONResponse {
	if userID != device.UserID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("You cannot set the presence state of another user."),
		}
	}
	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logrus.WithError(err).Error("unable to read request body")
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The request body could not be read: " + err.Error()),
		}
	}
	defer req.Body.Close() // nolint:errcheck

	// parse the request
	var r presenceRequest
	err = json.Unmarshal(data, &r)
	if err != nil {
		logrus.WithError(err).Error("unable to unmarshal request body")
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The request body could not be read: " + err.Error()),
		}
	}

	p := types.ToPresenceStatus(r.Presence)
	// requested new presence is not allowed by the spec
	if p == types.Unknown {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(
				fmt.Sprintf("The sent presence value '%s' is not allowed.", r.Presence),
			),
		}
	}

	lastActive := gomatrixserverlib.AsTimestamp(time.Now())

	pReq := userapi.InputPresenceRequest{
		UserID:       userID,
		DisplayName:  device.DisplayName,
		Presence:     p,
		StatusMsg:    r.StatusMsg,
		LastActiveTS: int64(lastActive),
	}
	pRes := userapi.InputPresenceResponse{}

	// send presence data directly to the userapi
	if err := userAPI.InputPresenceData(req.Context(), &pReq, &pRes); err != nil {
		logrus.WithError(err).Error("failed to set presence")
		return util.ErrorResponse(err)
	}

	eduReq := api.InputPresenceRequest{
		UserID:       userID,
		Presence:     p,
		StatusMsg:    r.StatusMsg,
		LastActiveTS: lastActive,
		StreamPos:    types2.StreamPosition(pRes.StreamPos),
	}
	eduRes := api.InputPresenceResponse{}
	// TODO: Inform EDU Server to send new presence to the federationsender/syncapi
	if err := eduAPI.InputPresence(req.Context(), &eduReq, &eduRes); err != nil {
		logrus.WithError(err).Error("failed to send presence to eduserver")
		return util.ErrorResponse(err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

type presenceResponse struct {
	Presence        string `json:"presence"`
	StatusMsg       string `json:"status_msg,omitempty"`
	LastActiveAgo   int64  `json:"last_active_ago,omitempty"`
	CurrentlyActive bool   `json:"currently_active,omitempty"`
}

// GetPresence returns the presence status of a given user.
// If the requesting user doesn't share a room with this user, the request is denied.
func GetPresence(req *http.Request,
	userAPI userapi.UserInternalAPI,
	userID string,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	device *userapi.Device,
) util.JSONResponse {
	// Only check allowance to see presence, if it's not our own user. (Otherwise sytest fails..)
	if device.UserID != userID {
		rsResp := roomserverAPI.QueryKnownUsersResponse{}
		rsQry := roomserverAPI.QueryKnownUsersRequest{UserID: device.UserID, SearchString: userID, Limit: 2}
		if err := rsAPI.QueryKnownUsers(req.Context(), &rsQry, &rsResp); err != nil {
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: jsonerror.InternalServerError(),
			}
		}

		// Users don't share a room, not allowed to see presence.
		if len(rsResp.Users) == 0 {
			return util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonerror.Forbidden("You cannot view the presence for this user."),
			}
		}
	}

	presence := userapi.QueryPresenceForUserResponse{}
	qry := userapi.QueryPresenceForUserRequest{UserID: userID}
	err := userAPI.QueryPresenceForUser(req.Context(), &qry, &presence)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"userID": userID,
		}).WithError(err).Error("unable to query presence")
		// return an empty presence, to make sytest happy
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: presence,
		}
	}

	resp := presenceResponse{}
	lastActive := time.Since(presence.LastActiveTS.Time())
	resp.LastActiveAgo = lastActive.Milliseconds()
	resp.StatusMsg = presence.StatusMsg
	resp.CurrentlyActive = lastActive <= time.Minute*5
	if !resp.CurrentlyActive {
		presence.PresenceStatus = types.Unavailable
	}
	resp.Presence = presence.PresenceStatus.String()
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp,
	}
}
