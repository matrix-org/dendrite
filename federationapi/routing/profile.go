// Copyright 2017 New Vector Ltd
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
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

// GetProfile implements GET /_matrix/federation/v1/query/profile
func GetProfile(
	httpReq *http.Request,
	userAPI userapi.FederationUserAPI,
	cfg *config.FederationAPI,
) util.JSONResponse {
	userID, field := httpReq.FormValue("user_id"), httpReq.FormValue("field")

	// httpReq.FormValue will return an empty string if value is not found
	if userID == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("The request body did not contain required argument 'user_id'."),
		}
	}

	_, domain, err := cfg.Matrix.SplitLocalID('@', userID)
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue(fmt.Sprintf("Domain %q does not match this server", domain)),
		}
	}

	var profileRes userapi.QueryProfileResponse
	err = userAPI.QueryProfile(httpReq.Context(), &userapi.QueryProfileRequest{
		UserID: userID,
	}, &profileRes)
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("userAPI.QueryProfile failed")
		return jsonerror.InternalServerError()
	}

	var res interface{}
	code := http.StatusOK

	if field != "" {
		switch field {
		case "displayname":
			res = eventutil.DisplayName{
				DisplayName: profileRes.DisplayName,
			}
		case "avatar_url":
			res = eventutil.AvatarURL{
				AvatarURL: profileRes.AvatarURL,
			}
		default:
			code = http.StatusBadRequest
			res = jsonerror.InvalidArgumentValue("The request body did not contain an allowed value of argument 'field'. Allowed values are either: 'avatar_url', 'displayname'.")
		}
	} else {
		res = eventutil.ProfileResponse{
			AvatarURL:   profileRes.AvatarURL,
			DisplayName: profileRes.DisplayName,
		}
	}

	return util.JSONResponse{
		Code: code,
		JSON: res,
	}
}
