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
	"fmt"
	"net/http"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/threepid"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"

	"github.com/matrix-org/util"
)

func SendBan(
	req *http.Request, profileAPI userapi.ClientUserAPI, device *userapi.Device,
	roomID string, cfg *config.ClientAPI,
	rsAPI roomserverAPI.ClientRoomserverAPI, asAPI appserviceAPI.AppServiceInternalAPI,
) util.JSONResponse {
	body, evTime, reqErr := extractRequestData(req)
	if reqErr != nil {
		return *reqErr
	}

	if body.UserID == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("missing user_id"),
		}
	}

	errRes := checkMemberInRoom(req.Context(), rsAPI, device.UserID, roomID)
	if errRes != nil {
		return *errRes
	}

	pl, errRes := getPowerlevels(req, rsAPI, roomID)
	if errRes != nil {
		return *errRes
	}
	allowedToBan := pl.UserLevel(device.UserID) >= pl.Ban
	if !allowedToBan {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("You don't have permission to ban this user, power level too low."),
		}
	}

	return sendMembership(req.Context(), profileAPI, device, roomID, gomatrixserverlib.Ban, body.Reason, cfg, body.UserID, evTime, rsAPI, asAPI)
}

func sendMembership(ctx context.Context, profileAPI userapi.ClientUserAPI, device *userapi.Device,
	roomID, membership, reason string, cfg *config.ClientAPI, targetUserID string, evTime time.Time,
	rsAPI roomserverAPI.ClientRoomserverAPI, asAPI appserviceAPI.AppServiceInternalAPI) util.JSONResponse {

	event, err := buildMembershipEvent(
		ctx, targetUserID, reason, profileAPI, device, membership,
		roomID, false, cfg, evTime, rsAPI, asAPI,
	)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("buildMembershipEvent failed")
		return jsonerror.InternalServerError()
	}

	serverName := device.UserDomain()
	if err = roomserverAPI.SendEvents(
		ctx, rsAPI,
		roomserverAPI.KindNew,
		[]*gomatrixserverlib.HeaderedEvent{event},
		device.UserDomain(),
		serverName,
		serverName,
		nil,
		false,
	); err != nil {
		util.GetLogger(ctx).WithError(err).Error("SendEvents failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

func SendKick(
	req *http.Request, profileAPI userapi.ClientUserAPI, device *userapi.Device,
	roomID string, cfg *config.ClientAPI,
	rsAPI roomserverAPI.ClientRoomserverAPI, asAPI appserviceAPI.AppServiceInternalAPI,
) util.JSONResponse {
	body, evTime, reqErr := extractRequestData(req)
	if reqErr != nil {
		return *reqErr
	}
	if body.UserID == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("missing user_id"),
		}
	}

	errRes := checkMemberInRoom(req.Context(), rsAPI, device.UserID, roomID)
	if errRes != nil {
		return *errRes
	}

	pl, errRes := getPowerlevels(req, rsAPI, roomID)
	if errRes != nil {
		return *errRes
	}
	allowedToKick := pl.UserLevel(device.UserID) >= pl.Kick
	if !allowedToKick {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("You don't have permission to kick this user, power level too low."),
		}
	}

	var queryRes roomserverAPI.QueryMembershipForUserResponse
	err := rsAPI.QueryMembershipForUser(req.Context(), &roomserverAPI.QueryMembershipForUserRequest{
		RoomID: roomID,
		UserID: body.UserID,
	}, &queryRes)
	if err != nil {
		return util.ErrorResponse(err)
	}
	// kick is only valid if the user is not currently banned or left (that is, they are joined or invited)
	if queryRes.Membership != gomatrixserverlib.Join && queryRes.Membership != gomatrixserverlib.Invite {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Unknown("cannot /kick banned or left users"),
		}
	}
	// TODO: should we be using SendLeave instead?
	return sendMembership(req.Context(), profileAPI, device, roomID, gomatrixserverlib.Leave, body.Reason, cfg, body.UserID, evTime, rsAPI, asAPI)
}

func SendUnban(
	req *http.Request, profileAPI userapi.ClientUserAPI, device *userapi.Device,
	roomID string, cfg *config.ClientAPI,
	rsAPI roomserverAPI.ClientRoomserverAPI, asAPI appserviceAPI.AppServiceInternalAPI,
) util.JSONResponse {
	body, evTime, reqErr := extractRequestData(req)
	if reqErr != nil {
		return *reqErr
	}
	if body.UserID == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("missing user_id"),
		}
	}

	errRes := checkMemberInRoom(req.Context(), rsAPI, device.UserID, roomID)
	if errRes != nil {
		return *errRes
	}

	var queryRes roomserverAPI.QueryMembershipForUserResponse
	err := rsAPI.QueryMembershipForUser(req.Context(), &roomserverAPI.QueryMembershipForUserRequest{
		RoomID: roomID,
		UserID: body.UserID,
	}, &queryRes)
	if err != nil {
		return util.ErrorResponse(err)
	}

	// unban is only valid if the user is currently banned
	if queryRes.Membership != gomatrixserverlib.Ban {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown("can only /unban users that are banned"),
		}
	}
	// TODO: should we be using SendLeave instead?
	return sendMembership(req.Context(), profileAPI, device, roomID, gomatrixserverlib.Leave, body.Reason, cfg, body.UserID, evTime, rsAPI, asAPI)
}

func SendInvite(
	req *http.Request, profileAPI userapi.ClientUserAPI, device *userapi.Device,
	roomID string, cfg *config.ClientAPI,
	rsAPI roomserverAPI.ClientRoomserverAPI, asAPI appserviceAPI.AppServiceInternalAPI,
) util.JSONResponse {
	body, evTime, reqErr := extractRequestData(req)
	if reqErr != nil {
		return *reqErr
	}

	inviteStored, jsonErrResp := checkAndProcessThreepid(
		req, device, body, cfg, rsAPI, profileAPI, roomID, evTime,
	)
	if jsonErrResp != nil {
		return *jsonErrResp
	}

	// If an invite has been stored on an identity server, it means that a
	// m.room.third_party_invite event has been emitted and that we shouldn't
	// emit a m.room.member one.
	if inviteStored {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}

	if body.UserID == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("missing user_id"),
		}
	}

	errRes := checkMemberInRoom(req.Context(), rsAPI, device.UserID, roomID)
	if errRes != nil {
		return *errRes
	}

	// We already received the return value, so no need to check for an error here.
	response, _ := sendInvite(req.Context(), profileAPI, device, roomID, body.UserID, body.Reason, cfg, rsAPI, asAPI, evTime)
	return response
}

// sendInvite sends an invitation to a user. Returns a JSONResponse and an error
func sendInvite(
	ctx context.Context,
	profileAPI userapi.ClientUserAPI,
	device *userapi.Device,
	roomID, userID, reason string,
	cfg *config.ClientAPI,
	rsAPI roomserverAPI.ClientRoomserverAPI,
	asAPI appserviceAPI.AppServiceInternalAPI, evTime time.Time,
) (util.JSONResponse, error) {
	event, err := buildMembershipEvent(
		ctx, userID, reason, profileAPI, device, gomatrixserverlib.Invite,
		roomID, false, cfg, evTime, rsAPI, asAPI,
	)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("buildMembershipEvent failed")
		return jsonerror.InternalServerError(), err
	}

	var inviteRes api.PerformInviteResponse
	if err := rsAPI.PerformInvite(ctx, &api.PerformInviteRequest{
		Event:           event,
		InviteRoomState: nil, // ask the roomserver to draw up invite room state for us
		RoomVersion:     event.RoomVersion,
		SendAsServer:    string(device.UserDomain()),
	}, &inviteRes); err != nil {
		util.GetLogger(ctx).WithError(err).Error("PerformInvite failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.InternalServerError(),
		}, err
	}
	if inviteRes.Error != nil {
		return inviteRes.Error.JSONResponse(), inviteRes.Error
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}, nil
}

func buildMembershipEvent(
	ctx context.Context,
	targetUserID, reason string, profileAPI userapi.ClientUserAPI,
	device *userapi.Device,
	membership, roomID string, isDirect bool,
	cfg *config.ClientAPI, evTime time.Time,
	rsAPI roomserverAPI.ClientRoomserverAPI, asAPI appserviceAPI.AppServiceInternalAPI,
) (*gomatrixserverlib.HeaderedEvent, error) {
	profile, err := loadProfile(ctx, targetUserID, cfg, profileAPI, asAPI)
	if err != nil {
		return nil, err
	}

	builder := gomatrixserverlib.EventBuilder{
		Sender:   device.UserID,
		RoomID:   roomID,
		Type:     "m.room.member",
		StateKey: &targetUserID,
	}

	content := gomatrixserverlib.MemberContent{
		Membership:  membership,
		DisplayName: profile.DisplayName,
		AvatarURL:   profile.AvatarURL,
		Reason:      reason,
		IsDirect:    isDirect,
	}

	if err = builder.SetContent(content); err != nil {
		return nil, err
	}

	identity, err := cfg.Matrix.SigningIdentityFor(device.UserDomain())
	if err != nil {
		return nil, err
	}

	return eventutil.QueryAndBuildEvent(ctx, &builder, cfg.Matrix, identity, evTime, rsAPI, nil)
}

// loadProfile lookups the profile of a given user from the database and returns
// it if the user is local to this server, or returns an empty profile if not.
// Returns an error if the retrieval failed or if the first parameter isn't a
// valid Matrix ID.
func loadProfile(
	ctx context.Context,
	userID string,
	cfg *config.ClientAPI,
	profileAPI userapi.ClientUserAPI,
	asAPI appserviceAPI.AppServiceInternalAPI,
) (*authtypes.Profile, error) {
	_, serverName, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return nil, err
	}

	var profile *authtypes.Profile
	if cfg.Matrix.IsLocalServerName(serverName) {
		profile, err = appserviceAPI.RetrieveUserProfile(ctx, userID, asAPI, profileAPI)
	} else {
		profile = &authtypes.Profile{}
	}

	return profile, err
}

func extractRequestData(req *http.Request) (body *threepid.MembershipRequest, evTime time.Time, resErr *util.JSONResponse) {

	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		resErr = reqErr
		return
	}

	evTime, err := httputil.ParseTSParam(req)
	if err != nil {
		resErr = &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue(err.Error()),
		}
		return
	}
	return
}

func checkAndProcessThreepid(
	req *http.Request,
	device *userapi.Device,
	body *threepid.MembershipRequest,
	cfg *config.ClientAPI,
	rsAPI roomserverAPI.ClientRoomserverAPI,
	profileAPI userapi.ClientUserAPI,
	roomID string,
	evTime time.Time,
) (inviteStored bool, errRes *util.JSONResponse) {

	inviteStored, err := threepid.CheckAndProcessInvite(
		req.Context(), device, body, cfg, rsAPI, profileAPI,
		roomID, evTime,
	)
	if err == threepid.ErrMissingParameter {
		return inviteStored, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(err.Error()),
		}
	} else if err == threepid.ErrNotTrusted {
		return inviteStored, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotTrusted(body.IDServer),
		}
	} else if err == eventutil.ErrRoomNoExists {
		return inviteStored, &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound(err.Error()),
		}
	} else if e, ok := err.(gomatrixserverlib.BadJSONError); ok {
		return inviteStored, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(e.Error()),
		}
	}
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("threepid.CheckAndProcessInvite failed")
		er := jsonerror.InternalServerError()
		return inviteStored, &er
	}
	return
}

func checkMemberInRoom(ctx context.Context, rsAPI roomserverAPI.ClientRoomserverAPI, userID, roomID string) *util.JSONResponse {
	var membershipRes roomserverAPI.QueryMembershipForUserResponse
	err := rsAPI.QueryMembershipForUser(ctx, &roomserverAPI.QueryMembershipForUserRequest{
		RoomID: roomID,
		UserID: userID,
	}, &membershipRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("QueryMembershipForUser: could not query membership for user")
		e := jsonerror.InternalServerError()
		return &e
	}
	if !membershipRes.IsInRoom {
		return &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("user does not belong to room"),
		}
	}
	return nil
}

func SendForget(
	req *http.Request, device *userapi.Device,
	roomID string, rsAPI roomserverAPI.ClientRoomserverAPI,
) util.JSONResponse {
	ctx := req.Context()
	logger := util.GetLogger(ctx).WithField("roomID", roomID).WithField("userID", device.UserID)
	var membershipRes roomserverAPI.QueryMembershipForUserResponse
	membershipReq := roomserverAPI.QueryMembershipForUserRequest{
		RoomID: roomID,
		UserID: device.UserID,
	}
	err := rsAPI.QueryMembershipForUser(ctx, &membershipReq, &membershipRes)
	if err != nil {
		logger.WithError(err).Error("QueryMembershipForUser: could not query membership for user")
		return jsonerror.InternalServerError()
	}
	if !membershipRes.RoomExists {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("room does not exist"),
		}
	}
	if membershipRes.IsInRoom {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown(fmt.Sprintf("User %s is in room %s", device.UserID, roomID)),
		}
	}

	request := roomserverAPI.PerformForgetRequest{
		RoomID: roomID,
		UserID: device.UserID,
	}
	response := roomserverAPI.PerformForgetResponse{}
	if err := rsAPI.PerformForget(ctx, &request, &response); err != nil {
		logger.WithError(err).Error("PerformForget: unable to forget room")
		return jsonerror.InternalServerError()
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

func getPowerlevels(req *http.Request, rsAPI roomserverAPI.ClientRoomserverAPI, roomID string) (*gomatrixserverlib.PowerLevelContent, *util.JSONResponse) {
	plEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomPowerLevels,
		StateKey:  "",
	})
	if plEvent == nil {
		return nil, &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("You don't have permission to perform this action, no power_levels event in this room."),
		}
	}
	pl, err := plEvent.PowerLevels()
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("You don't have permission to perform this action, the power_levels event for this room is malformed so auth checks cannot be performed."),
		}
	}
	return pl, nil
}
