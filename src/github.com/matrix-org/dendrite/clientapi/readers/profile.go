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
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"

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

// GetProfile implements GET /profile/{userID}
func GetProfile(
	req *http.Request, accountDB *accounts.Database, userID string,
) util.JSONResponse {
	if req.Method != "GET" {
		return util.JSONResponse{
			Code: 405,
			JSON: jsonerror.NotFound("Bad method"),
		}
	}
	localpart := getLocalPart(userID)
	profile, err := accountDB.GetProfileByLocalpart(localpart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}
	res := profileResponse{
		AvatarURL:   profile.AvatarURL,
		DisplayName: profile.DisplayName,
	}
	return util.JSONResponse{
		Code: 200,
		JSON: res,
	}
}

// GetAvatarURL implements GET /profile/{userID}/avatar_url
func GetAvatarURL(
	req *http.Request, accountDB *accounts.Database, userID string,
) util.JSONResponse {
	localpart := getLocalPart(userID)
	profile, err := accountDB.GetProfileByLocalpart(localpart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}
	res := avatarURL{
		AvatarURL: profile.AvatarURL,
	}
	return util.JSONResponse{
		Code: 200,
		JSON: res,
	}
}

// SetAvatarURL implements PUT /profile/{userID}/avatar_url
func SetAvatarURL(
	req *http.Request, accountDB *accounts.Database, userID string,
	producer *producers.UserUpdateProducer,
) util.JSONResponse {
	var r avatarURL
	if resErr := httputil.UnmarshalJSONRequest(req, &r); resErr != nil {
		return *resErr
	}
	if r.AvatarURL == "" {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("'avatar_url' must be supplied."),
		}
	}

	localpart := getLocalPart(userID)

	oldProfile, err := accountDB.GetProfileByLocalpart(localpart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	if err := accountDB.SetAvatarURL(localpart, r.AvatarURL); err != nil {
		return httputil.LogThenError(req, err)
	}

	if err := producer.SendUpdate(userID, "avatar_url", oldProfile.AvatarURL, r.AvatarURL); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}

// GetDisplayName implements GET /profile/{userID}/displayname
func GetDisplayName(
	req *http.Request, accountDB *accounts.Database, userID string,
) util.JSONResponse {
	localpart := getLocalPart(userID)
	profile, err := accountDB.GetProfileByLocalpart(localpart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}
	res := displayName{
		DisplayName: profile.DisplayName,
	}
	return util.JSONResponse{
		Code: 200,
		JSON: res,
	}
}

// SetDisplayName implements PUT /profile/{userID}/displayname
func SetDisplayName(
	req *http.Request, accountDB *accounts.Database, userID string,
	producer *producers.UserUpdateProducer,
) util.JSONResponse {
	var r displayName
	if resErr := httputil.UnmarshalJSONRequest(req, &r); resErr != nil {
		return *resErr
	}
	if r.DisplayName == "" {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("'displayname' must be supplied."),
		}
	}

	localpart := getLocalPart(userID)

	oldProfile, err := accountDB.GetProfileByLocalpart(localpart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	if err := accountDB.SetDisplayName(localpart, r.DisplayName); err != nil {
		return httputil.LogThenError(req, err)
	}

	if err := producer.SendUpdate(userID, "displayname", oldProfile.DisplayName, r.DisplayName); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
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
