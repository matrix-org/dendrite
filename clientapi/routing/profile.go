// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"

	appserviceAPI "github.com/element-hq/dendrite/appservice/api"
	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
	"github.com/element-hq/dendrite/clientapi/httputil"
	"github.com/element-hq/dendrite/internal/eventutil"
	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/setup/config"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/util"
)

// GetProfile implements GET /profile/{userID}
func GetProfile(
	req *http.Request, profileAPI userapi.ProfileAPI, cfg *config.ClientAPI,
	userID string,
	asAPI appserviceAPI.AppServiceInternalAPI,
	federation fclient.FederationClient,
) util.JSONResponse {
	profile, err := getProfile(req.Context(), profileAPI, cfg, userID, asAPI, federation)
	if err != nil {
		if err == appserviceAPI.ErrProfileNotExists {
			return util.JSONResponse{
				Code: http.StatusNotFound,
				JSON: spec.NotFound("The user does not exist or does not have a profile"),
			}
		}

		util.GetLogger(req.Context()).WithError(err).Error("getProfile failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: eventutil.UserProfile{
			AvatarURL:   profile.AvatarURL,
			DisplayName: profile.DisplayName,
		},
	}
}

// GetAvatarURL implements GET /profile/{userID}/avatar_url
func GetAvatarURL(
	req *http.Request, profileAPI userapi.ProfileAPI, cfg *config.ClientAPI,
	userID string, asAPI appserviceAPI.AppServiceInternalAPI,
	federation fclient.FederationClient,
) util.JSONResponse {
	profile := GetProfile(req, profileAPI, cfg, userID, asAPI, federation)
	p, ok := profile.JSON.(eventutil.UserProfile)
	// not a profile response, so most likely an error, return that
	if !ok {
		return profile
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: eventutil.UserProfile{
			AvatarURL: p.AvatarURL,
		},
	}
}

// SetAvatarURL implements PUT /profile/{userID}/avatar_url
func SetAvatarURL(
	req *http.Request, profileAPI userapi.ProfileAPI,
	device *userapi.Device, userID string, cfg *config.ClientAPI, rsAPI api.ClientRoomserverAPI,
) util.JSONResponse {
	if userID != device.UserID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("userID does not match the current user"),
		}
	}

	var r eventutil.UserProfile
	if resErr := httputil.UnmarshalJSONRequest(req, &r); resErr != nil {
		return *resErr
	}

	localpart, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	if !cfg.Matrix.IsLocalServerName(domain) {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("userID does not belong to a locally configured domain"),
		}
	}

	evTime, err := httputil.ParseTSParam(req)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam(err.Error()),
		}
	}

	profile, changed, err := profileAPI.SetAvatarURL(req.Context(), localpart, domain, r.AvatarURL)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("profileAPI.SetAvatarURL failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	// No need to build new membership events, since nothing changed
	if !changed {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}

	response, err := updateProfile(req.Context(), rsAPI, device, profile, userID, evTime)
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
	req *http.Request, profileAPI userapi.ProfileAPI, cfg *config.ClientAPI,
	userID string, asAPI appserviceAPI.AppServiceInternalAPI,
	federation fclient.FederationClient,
) util.JSONResponse {
	profile := GetProfile(req, profileAPI, cfg, userID, asAPI, federation)
	p, ok := profile.JSON.(eventutil.UserProfile)
	// not a profile response, so most likely an error, return that
	if !ok {
		return profile
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: eventutil.UserProfile{
			DisplayName: p.DisplayName,
		},
	}
}

// SetDisplayName implements PUT /profile/{userID}/displayname
func SetDisplayName(
	req *http.Request, profileAPI userapi.ProfileAPI,
	device *userapi.Device, userID string, cfg *config.ClientAPI, rsAPI api.ClientRoomserverAPI,
) util.JSONResponse {
	if userID != device.UserID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("userID does not match the current user"),
		}
	}

	var r eventutil.UserProfile
	if resErr := httputil.UnmarshalJSONRequest(req, &r); resErr != nil {
		return *resErr
	}

	localpart, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	if !cfg.Matrix.IsLocalServerName(domain) {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("userID does not belong to a locally configured domain"),
		}
	}

	evTime, err := httputil.ParseTSParam(req)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam(err.Error()),
		}
	}

	profile, changed, err := profileAPI.SetDisplayName(req.Context(), localpart, domain, r.DisplayName)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("profileAPI.SetDisplayName failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	// No need to build new membership events, since nothing changed
	if !changed {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}

	response, err := updateProfile(req.Context(), rsAPI, device, profile, userID, evTime)
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
	userID string, evTime time.Time,
) (util.JSONResponse, error) {
	deviceUserID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.Unknown("internal server error"),
		}, err
	}

	rooms, err := rsAPI.QueryRoomsForUser(ctx, *deviceUserID, "join")
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("QueryRoomsForUser failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}, err
	}

	roomIDStrs := make([]string, len(rooms))
	for i, room := range rooms {
		roomIDStrs[i] = room.String()
	}

	_, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}, err
	}

	events, err := buildMembershipEvents(
		ctx, roomIDStrs, *profile, userID, evTime, rsAPI,
	)
	switch e := err.(type) {
	case nil:
	case gomatrixserverlib.BadJSONError:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON(e.Error()),
		}, e
	default:
		util.GetLogger(ctx).WithError(err).Error("buildMembershipEvents failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}, e
	}

	if err := api.SendEvents(ctx, rsAPI, api.KindNew, events, device.UserDomain(), domain, domain, nil, false); err != nil {
		util.GetLogger(ctx).WithError(err).Error("SendEvents failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}, err
	}
	return util.JSONResponse{}, nil
}

// getProfile gets the full profile of a user by querying the database or a
// remote homeserver.
// Returns an error when something goes wrong or specifically
// eventutil.ErrProfileNotExists when the profile doesn't exist.
func getProfile(
	ctx context.Context, profileAPI userapi.ProfileAPI, cfg *config.ClientAPI,
	userID string,
	asAPI appserviceAPI.AppServiceInternalAPI,
	federation fclient.FederationClient,
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
					return nil, appserviceAPI.ErrProfileNotExists
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
	roomIDs []string,
	newProfile authtypes.Profile, userID string,
	evTime time.Time, rsAPI api.ClientRoomserverAPI,
) ([]*types.HeaderedEvent, error) {
	evs := []*types.HeaderedEvent{}

	fullUserID, err := spec.NewUserID(userID, true)
	if err != nil {
		return nil, err
	}
	for _, roomID := range roomIDs {
		validRoomID, err := spec.NewRoomID(roomID)
		if err != nil {
			return nil, err
		}
		senderID, err := rsAPI.QuerySenderIDForUser(ctx, *validRoomID, *fullUserID)
		if err != nil {
			return nil, err
		} else if senderID == nil {
			return nil, fmt.Errorf("sender ID not found for %s in %s", *fullUserID, *validRoomID)
		}
		senderIDString := string(*senderID)
		proto := gomatrixserverlib.ProtoEvent{
			SenderID: senderIDString,
			RoomID:   roomID,
			Type:     "m.room.member",
			StateKey: &senderIDString,
		}

		content := gomatrixserverlib.MemberContent{
			Membership: spec.Join,
		}

		content.DisplayName = newProfile.DisplayName
		content.AvatarURL = newProfile.AvatarURL

		if err = proto.SetContent(content); err != nil {
			return nil, err
		}

		user, err := spec.NewUserID(userID, true)
		if err != nil {
			return nil, err
		}

		identity, err := rsAPI.SigningIdentityFor(ctx, *validRoomID, *user)
		if err != nil {
			return nil, err
		}

		event, err := eventutil.QueryAndBuildEvent(ctx, &proto, &identity, evTime, rsAPI, nil)
		if err != nil {
			return nil, err
		}

		evs = append(evs, event)
	}

	return evs, nil
}
