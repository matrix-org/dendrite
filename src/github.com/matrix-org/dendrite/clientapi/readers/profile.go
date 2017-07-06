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

package readers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	// "github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	// "github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type profileResponse struct {
	AvatarURL   string `json:"avatar_url"`
	DisplayName string `json:"displayname"`
}

func Profile(
	req *http.Request, accountDB *accounts.Database, userID string,
) util.JSONResponse {
	if req.Method == "GET" {
		localpart := getLocalPart(userID)
		profile, err := accountDB.GetProfileByLocalpart(localpart)
		if err == nil {
			res := profileResponse{
				AvatarURL:   profile.AvatarURL,
				DisplayName: profile.DisplayName,
			}
			return util.JSONResponse{
				Code: 200,
				JSON: res,
			}
		}
		return util.JSONResponse{
			Code: 500,
			JSON: jsonerror.Unknown("Failed to load user profile"),
		}
	}
	return util.JSONResponse{
		Code: 405,
		JSON: jsonerror.NotFound("Bad method"),
	}
}

func getLocalPart(userID string) string {
	if !strings.HasPrefix(userID, "@") {
		panic(fmt.Errorf("Invalid user ID"))
	}

	// Get the part before ":"
	username := strings.Split(userID, ":")[0]
	// Return the part after the "@"
	return strings.Split(username, "@")[1]
}
