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
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/events"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
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
	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

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
	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

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
	req *http.Request, accountDB *accounts.Database, device *authtypes.Device,
	userID string, producer *producers.UserUpdateProducer, cfg *config.Dendrite,
	rsProducer *producers.RoomserverProducer, queryAPI api.RoomserverQueryAPI,
) util.JSONResponse {
	if userID != device.UserID {
		return util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("userID does not match the current user"),
		}
	}

	changedKey := "avatar_url"

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

	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	oldProfile, err := accountDB.GetProfileByLocalpart(localpart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	if err = accountDB.SetAvatarURL(localpart, r.AvatarURL); err != nil {
		return httputil.LogThenError(req, err)
	}

	memberships, err := accountDB.GetMembershipsByLocalpart(localpart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	newProfile := authtypes.Profile{
		Localpart:   localpart,
		DisplayName: oldProfile.DisplayName,
		AvatarURL:   r.AvatarURL,
	}

	events, err := buildMembershipEvents(memberships, accountDB, newProfile, userID, cfg, queryAPI)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	if err := rsProducer.SendEvents(events, cfg.Matrix.ServerName); err != nil {
		return httputil.LogThenError(req, err)
	}

	if err := producer.SendUpdate(userID, changedKey, oldProfile.AvatarURL, r.AvatarURL); err != nil {
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
	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

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
	req *http.Request, accountDB *accounts.Database, device *authtypes.Device,
	userID string, producer *producers.UserUpdateProducer, cfg *config.Dendrite,
	rsProducer *producers.RoomserverProducer, queryAPI api.RoomserverQueryAPI,
) util.JSONResponse {
	if userID != device.UserID {
		return util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("userID does not match the current user"),
		}
	}

	changedKey := "displayname"

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

	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	oldProfile, err := accountDB.GetProfileByLocalpart(localpart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	if err = accountDB.SetDisplayName(localpart, r.DisplayName); err != nil {
		return httputil.LogThenError(req, err)
	}

	memberships, err := accountDB.GetMembershipsByLocalpart(localpart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	newProfile := authtypes.Profile{
		Localpart:   localpart,
		DisplayName: r.DisplayName,
		AvatarURL:   oldProfile.AvatarURL,
	}

	events, err := buildMembershipEvents(memberships, accountDB, newProfile, userID, cfg, queryAPI)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	if err := rsProducer.SendEvents(events, cfg.Matrix.ServerName); err != nil {
		return httputil.LogThenError(req, err)
	}

	if err := producer.SendUpdate(userID, changedKey, oldProfile.DisplayName, r.DisplayName); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}

func buildMembershipEvents(
	memberships []authtypes.Membership, db *accounts.Database,
	newProfile authtypes.Profile, userID string, cfg *config.Dendrite,
	queryAPI api.RoomserverQueryAPI,
) ([]gomatrixserverlib.Event, error) {
	evs := []gomatrixserverlib.Event{}

	for _, membership := range memberships {
		builder := gomatrixserverlib.EventBuilder{
			Sender:   userID,
			RoomID:   membership.RoomID,
			Type:     "m.room.member",
			StateKey: &userID,
		}

		content := events.MemberContent{
			Membership: "join",
		}

		content.DisplayName = newProfile.DisplayName
		content.AvatarURL = newProfile.AvatarURL

		if err := builder.SetContent(content); err != nil {
			return nil, err
		}

		event, err := events.BuildEvent(&builder, *cfg, queryAPI, nil)
		if err != nil {
			return nil, err
		}

		evs = append(evs, *event)
	}

	return evs, nil
}
