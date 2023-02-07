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
	"net/http"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"

	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/util"
)

// GetProfile implements GET /profile/{userID}
func GetProfile(
	req *http.Request, profileAPI userapi.ClientUserAPI, cfg *config.ClientAPI,
	userID string,
	asAPI appserviceAPI.AppServiceInternalAPI,
	federation *gomatrixserverlib.FederationClient,
) util.JSONResponse {
	profile, err := getProfile(req.Context(), profileAPI, cfg, userID, asAPI, federation)
	if err != nil {
		if err == eventutil.ErrProfileNoExists {
			return util.JSONResponse{
				Code: http.StatusNotFound,
				JSON: jsonerror.NotFound("The user does not exist or does not have a profile"),
			}
		}

		util.GetLogger(req.Context()).WithError(err).Error("getProfile failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: eventutil.ProfileResponse{
			AvatarURL:   profile.AvatarURL,
			DisplayName: profile.DisplayName,
		},
	}
}

// GetAvatarURL implements GET /profile/{userID}/avatar_url
func GetAvatarURL(
	req *http.Request, profileAPI userapi.ClientUserAPI, cfg *config.ClientAPI,
	userID string, asAPI appserviceAPI.AppServiceInternalAPI,
	federation *gomatrixserverlib.FederationClient,
) util.JSONResponse {
	profile, err := getProfile(req.Context(), profileAPI, cfg, userID, asAPI, federation)
	if err != nil {
		if err == eventutil.ErrProfileNoExists {
			return util.JSONResponse{
				Code: http.StatusNotFound,
				JSON: jsonerror.NotFound("The user does not exist or does not have a profile"),
			}
		}

		util.GetLogger(req.Context()).WithError(err).Error("getProfile failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: eventutil.AvatarURL{
			AvatarURL: profile.AvatarURL,
		},
	}
}

// SetAvatarURL implements PUT /profile/{userID}/avatar_url
func SetAvatarURL(
	req *http.Request, profileAPI userapi.ClientUserAPI,
	device *userapi.Device, userID string, cfg *config.ClientAPI, rsAPI api.ClientRoomserverAPI,
) util.JSONResponse {
	if userID != device.UserID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("userID does not match the current user"),
		}
	}

	var r eventutil.AvatarURL
	if resErr := httputil.UnmarshalJSONRequest(req, &r); resErr != nil {
		return *resErr
	}
	if r.AvatarURL == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("'avatar_url' must be supplied."),
		}
	}

	localpart, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError()
	}

	if !cfg.Matrix.IsLocalServerName(domain) {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("userID does not belong to a locally configured domain"),
		}
	}

	evTime, err := httputil.ParseTSParam(req)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue(err.Error()),
		}
	}

	setRes := &userapi.PerformSetAvatarURLResponse{}
	if err = profileAPI.SetAvatarURL(req.Context(), &userapi.PerformSetAvatarURLRequest{
		Localpart:  localpart,
		ServerName: domain,
		AvatarURL:  r.AvatarURL,
	}, setRes); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("profileAPI.SetAvatarURL failed")
		return jsonerror.InternalServerError()
	}
	// No need to build new membership events, since nothing changed
	if !setRes.Changed {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}

	response, err := updateProfile(req.Context(), rsAPI, device, setRes.Profile, userID, cfg, evTime)
	if err != nil {
		return response
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// GetDisplayName implements GET /profile/{userID}/displayname
func GetDisplayName(
	req *http.Request, profileAPI userapi.ClientUserAPI, cfg *config.ClientAPI,
	userID string, asAPI appserviceAPI.AppServiceInternalAPI,
	federation *gomatrixserverlib.FederationClient,
) util.JSONResponse {
	profile, err := getProfile(req.Context(), profileAPI, cfg, userID, asAPI, federation)
	if err != nil {
		if err == eventutil.ErrProfileNoExists {
			return util.JSONResponse{
				Code: http.StatusNotFound,
				JSON: jsonerror.NotFound("The user does not exist or does not have a profile"),
			}
		}

		util.GetLogger(req.Context()).WithError(err).Error("getProfile failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: eventutil.DisplayName{
			DisplayName: profile.DisplayName,
		},
	}
}

// SetDisplayName implements PUT /profile/{userID}/displayname
func SetDisplayName(
	req *http.Request, profileAPI userapi.ClientUserAPI,
	device *userapi.Device, userID string, cfg *config.ClientAPI, rsAPI api.ClientRoomserverAPI,
) util.JSONResponse {
	if userID != device.UserID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("userID does not match the current user"),
		}
	}

	var r eventutil.DisplayName
	if resErr := httputil.UnmarshalJSONRequest(req, &r); resErr != nil {
		return *resErr
	}
	if r.DisplayName == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("'displayname' must be supplied."),
		}
	}

	localpart, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError()
	}

	if !cfg.Matrix.IsLocalServerName(domain) {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("userID does not belong to a locally configured domain"),
		}
	}

	evTime, err := httputil.ParseTSParam(req)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue(err.Error()),
		}
	}

	profileRes := &userapi.PerformUpdateDisplayNameResponse{}
	err = profileAPI.SetDisplayName(req.Context(), &userapi.PerformUpdateDisplayNameRequest{
		Localpart:   localpart,
		ServerName:  domain,
		DisplayName: r.DisplayName,
	}, profileRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("profileAPI.SetDisplayName failed")
		return jsonerror.InternalServerError()
	}
	// No need to build new membership events, since nothing changed
	if !profileRes.Changed {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}

	response, err := updateProfile(req.Context(), rsAPI, device, profileRes.Profile, userID, cfg, evTime)
	if err != nil {
		return response
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

func updateProfile(
	ctx context.Context, rsAPI api.ClientRoomserverAPI, device *userapi.Device,
	profile *authtypes.Profile,
	userID string, cfg *config.ClientAPI, evTime time.Time,
) (util.JSONResponse, error) {
	var res api.QueryRoomsForUserResponse
	err := rsAPI.QueryRoomsForUser(ctx, &api.QueryRoomsForUserRequest{
		UserID:         device.UserID,
		WantMembership: "join",
	}, &res)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("QueryRoomsForUser failed")
		return jsonerror.InternalServerError(), err
	}

	_, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError(), err
	}

	events, err := buildMembershipEvents(
		ctx, device, res.RoomIDs, *profile, userID, cfg, evTime, rsAPI,
	)
	switch e := err.(type) {
	case nil:
	case gomatrixserverlib.BadJSONError:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(e.Error()),
		}, e
	default:
		util.GetLogger(ctx).WithError(err).Error("buildMembershipEvents failed")
		return jsonerror.InternalServerError(), e
	}

	if err := api.SendEvents(ctx, rsAPI, api.KindNew, events, device.UserDomain(), domain, domain, nil, true); err != nil {
		util.GetLogger(ctx).WithError(err).Error("SendEvents failed")
		return jsonerror.InternalServerError(), err
	}
	return util.JSONResponse{}, nil
}

// getProfile gets the full profile of a user by querying the database or a
// remote homeserver.
// Returns an error when something goes wrong or specifically
// eventutil.ErrProfileNoExists when the profile doesn't exist.
func getProfile(
	ctx context.Context, profileAPI userapi.ClientUserAPI, cfg *config.ClientAPI,
	userID string,
	asAPI appserviceAPI.AppServiceInternalAPI,
	federation *gomatrixserverlib.FederationClient,
) (*authtypes.Profile, error) {
	localpart, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return nil, err
	}

	if !cfg.Matrix.IsLocalServerName(domain) {
		profile, fedErr := federation.LookupProfile(ctx, cfg.Matrix.ServerName, domain, userID, "")
		if fedErr != nil {
			if x, ok := fedErr.(gomatrix.HTTPError); ok {
				if x.Code == http.StatusNotFound {
					return nil, eventutil.ErrProfileNoExists
				}
			}

			return nil, fedErr
		}

		return &authtypes.Profile{
			Localpart:   localpart,
			DisplayName: profile.DisplayName,
			AvatarURL:   profile.AvatarURL,
		}, nil
	}

	profile, err := appserviceAPI.RetrieveUserProfile(ctx, userID, asAPI, profileAPI)
	if err != nil {
		return nil, err
	}

	return profile, nil
}

func buildMembershipEvents(
	ctx context.Context,
	device *userapi.Device,
	roomIDs []string,
	newProfile authtypes.Profile, userID string, cfg *config.ClientAPI,
	evTime time.Time, rsAPI api.ClientRoomserverAPI,
) ([]*gomatrixserverlib.HeaderedEvent, error) {
	evs := []*gomatrixserverlib.HeaderedEvent{}

	for _, roomID := range roomIDs {
		verReq := api.QueryRoomVersionForRoomRequest{RoomID: roomID}
		verRes := api.QueryRoomVersionForRoomResponse{}
		if err := rsAPI.QueryRoomVersionForRoom(ctx, &verReq, &verRes); err != nil {
			return nil, err
		}

		builder := gomatrixserverlib.EventBuilder{
			Sender:   userID,
			RoomID:   roomID,
			Type:     "m.room.member",
			StateKey: &userID,
		}

		content := gomatrixserverlib.MemberContent{
			Membership: gomatrixserverlib.Join,
		}

		content.DisplayName = newProfile.DisplayName
		content.AvatarURL = newProfile.AvatarURL

		if err := builder.SetContent(content); err != nil {
			return nil, err
		}

		identity, err := cfg.Matrix.SigningIdentityFor(device.UserDomain())
		if err != nil {
			return nil, err
		}

		event, err := eventutil.QueryAndBuildEvent(ctx, &builder, cfg.Matrix, identity, evTime, rsAPI, nil)
		if err != nil {
			return nil, err
		}

		evs = append(evs, event.Headered(verRes.RoomVersion))
	}

	return evs, nil
}
