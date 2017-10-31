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
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type profileResponse struct {
	AvatarURL   string `json:"avatar_url"`
	DisplayName string `json:"displayname"`
}

type avatarURL struct {
	AvatarURL string `json:"avatar_url"`
}

type displayName struct {
	DisplayName string `json:"displayname"`
}

// GetProfile implements /_matrix/federation/v1/query/profile
func GetProfile(
	httpReq *http.Request,
	accountDB *accounts.Database,
) util.JSONResponse {
	userID, field := httpReq.FormValue("user_id"), httpReq.FormValue("field")

	// httpReq.FormValue will return an empty string if value is not found
	if userID == "" {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.MissingArgument("The request body did not contain required argument 'user_id'."),
		}
	}

	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return httputil.LogThenError(httpReq, err)
	}

	profile, err := accountDB.GetProfileByLocalpart(httpReq.Context(), localpart)
	if err != nil {
		return httputil.LogThenError(httpReq, err)
	}

	var res interface{}
	code := 200

	if field != "" {
		switch field {
		case "displayname":
			res = displayName{
				profile.DisplayName,
			}
		case "avatar_url":
			res = avatarURL{
				profile.AvatarURL,
			}
		default:
			code = 400
			res = jsonerror.InvalidArgumentBody("The request body did not contain allowed values of argument 'field'. Allowed: 'avatar_url', 'displayname'.")
		}
	} else {
		res = profileResponse{
			profile.AvatarURL,
			profile.DisplayName,
		}
	}

	return util.JSONResponse{
		Code: code,
		JSON: res,
	}
}
