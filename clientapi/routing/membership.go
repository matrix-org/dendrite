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
	"crypto/ed25519"
	"fmt"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/threepid"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"

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
			JSON: spec.BadJSON("missing user_id"),
		}
	}

	deviceUserID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("You don't have permission to ban this user, bad userID"),
		}
	}
	validRoomID, err := spec.NewRoomID(roomID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("RoomID is invalid"),
		}
	}
	senderID, err := rsAPI.QuerySenderIDForUser(req.Context(), *validRoomID, *deviceUserID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("You don't have permission to ban this user, unknown senderID"),
		}
	}

	errRes := checkMemberInRoom(req.Context(), rsAPI, *deviceUserID, roomID)
	if errRes != nil {
		return *errRes
	}

	pl, errRes := getPowerlevels(req, rsAPI, roomID)
	if errRes != nil {
		return *errRes
	}
	allowedToBan := pl.UserLevel(senderID) >= pl.Ban
	if !allowedToBan {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("You don't have permission to ban this user, power level too low."),
		}
	}

	return sendMembership(req.Context(), profileAPI, device, roomID, spec.Ban, body.Reason, cfg, body.UserID, evTime, rsAPI, asAPI)
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
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	serverName := device.UserDomain()
	if err = roomserverAPI.SendEvents(
		ctx, rsAPI,
		roomserverAPI.KindNew,
		[]*types.HeaderedEvent{event},
		device.UserDomain(),
		serverName,
		serverName,
		nil,
		false,
	); err != nil {
		util.GetLogger(ctx).WithError(err).Error("SendEvents failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
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
			JSON: spec.BadJSON("missing user_id"),
		}
	}

	deviceUserID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("You don't have permission to kick this user, bad userID"),
		}
	}
	validRoomID, err := spec.NewRoomID(roomID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("RoomID is invalid"),
		}
	}
	senderID, err := rsAPI.QuerySenderIDForUser(req.Context(), *validRoomID, *deviceUserID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("You don't have permission to kick this user, unknown senderID"),
		}
	}

	errRes := checkMemberInRoom(req.Context(), rsAPI, *deviceUserID, roomID)
	if errRes != nil {
		return *errRes
	}

	pl, errRes := getPowerlevels(req, rsAPI, roomID)
	if errRes != nil {
		return *errRes
	}
	allowedToKick := pl.UserLevel(senderID) >= pl.Kick
	if !allowedToKick {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("You don't have permission to kick this user, power level too low."),
		}
	}

	bodyUserID, err := spec.NewUserID(body.UserID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("body userID is invalid"),
		}
	}
	var queryRes roomserverAPI.QueryMembershipForUserResponse
	err = rsAPI.QueryMembershipForUser(req.Context(), &roomserverAPI.QueryMembershipForUserRequest{
		RoomID: roomID,
		UserID: *bodyUserID,
	}, &queryRes)
	if err != nil {
		return util.ErrorResponse(err)
	}
	// kick is only valid if the user is not currently banned or left (that is, they are joined or invited)
	if queryRes.Membership != spec.Join && queryRes.Membership != spec.Invite {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Unknown("cannot /kick banned or left users"),
		}
	}
	// TODO: should we be using SendLeave instead?
	return sendMembership(req.Context(), profileAPI, device, roomID, spec.Leave, body.Reason, cfg, body.UserID, evTime, rsAPI, asAPI)
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
			JSON: spec.BadJSON("missing user_id"),
		}
	}

	deviceUserID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("You don't have permission to kick this user, bad userID"),
		}
	}

	errRes := checkMemberInRoom(req.Context(), rsAPI, *deviceUserID, roomID)
	if errRes != nil {
		return *errRes
	}

	bodyUserID, err := spec.NewUserID(body.UserID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("body userID is invalid"),
		}
	}
	var queryRes roomserverAPI.QueryMembershipForUserResponse
	err = rsAPI.QueryMembershipForUser(req.Context(), &roomserverAPI.QueryMembershipForUserRequest{
		RoomID: roomID,
		UserID: *bodyUserID,
	}, &queryRes)
	if err != nil {
		return util.ErrorResponse(err)
	}

	// unban is only valid if the user is currently banned
	if queryRes.Membership != spec.Ban {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.Unknown("can only /unban users that are banned"),
		}
	}
	// TODO: should we be using SendLeave instead?
	return sendMembership(req.Context(), profileAPI, device, roomID, spec.Leave, body.Reason, cfg, body.UserID, evTime, rsAPI, asAPI)
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
			JSON: spec.BadJSON("missing user_id"),
		}
	}

	deviceUserID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("You don't have permission to kick this user, bad userID"),
		}
	}

	errRes := checkMemberInRoom(req.Context(), rsAPI, *deviceUserID, roomID)
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
	validRoomID, err := spec.NewRoomID(roomID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam("RoomID is invalid"),
		}, err
	}
	inviter, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}, err
	}
	invitee, err := spec.NewUserID(userID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam("UserID is invalid"),
		}, err
	}
	profile, err := loadProfile(ctx, userID, cfg, profileAPI, asAPI)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}, err
	}
	identity, err := cfg.Matrix.SigningIdentityFor(device.UserDomain())
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}, err
	}
	err = rsAPI.PerformInvite(ctx, &api.PerformInviteRequest{
		InviteInput: roomserverAPI.InviteInput{
			RoomID:      *validRoomID,
			Inviter:     *inviter,
			Invitee:     *invitee,
			DisplayName: profile.DisplayName,
			AvatarURL:   profile.AvatarURL,
			Reason:      reason,
			IsDirect:    false,
			KeyID:       identity.KeyID,
			PrivateKey:  identity.PrivateKey,
			EventTime:   evTime,
		},
		InviteRoomState: nil, // ask the roomserver to draw up invite room state for us
		SendAsServer:    string(device.UserDomain()),
	})

	switch e := err.(type) {
	case roomserverAPI.ErrInvalidID:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.Unknown(e.Error()),
		}, e
	case roomserverAPI.ErrNotAllowed:
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden(e.Error()),
		}, e
	case nil:
	default:
		util.GetLogger(ctx).WithError(err).Error("PerformInvite failed")
		sentry.CaptureException(err)
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}, err
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}, nil
}

func buildMembershipEventDirect(
	ctx context.Context,
	targetSenderID spec.SenderID, reason string, userDisplayName, userAvatarURL string,
	sender spec.SenderID, senderDomain spec.ServerName,
	membership, roomID string, isDirect bool,
	keyID gomatrixserverlib.KeyID, privateKey ed25519.PrivateKey, evTime time.Time,
	rsAPI roomserverAPI.ClientRoomserverAPI,
) (*types.HeaderedEvent, error) {
	targetSenderString := string(targetSenderID)
	proto := gomatrixserverlib.ProtoEvent{
		SenderID: string(sender),
		RoomID:   roomID,
		Type:     "m.room.member",
		StateKey: &targetSenderString,
	}

	content := gomatrixserverlib.MemberContent{
		Membership:  membership,
		DisplayName: userDisplayName,
		AvatarURL:   userAvatarURL,
		Reason:      reason,
		IsDirect:    isDirect,
	}

	if err := proto.SetContent(content); err != nil {
		return nil, err
	}

	identity := &fclient.SigningIdentity{
		ServerName: senderDomain,
		KeyID:      keyID,
		PrivateKey: privateKey,
	}
	return eventutil.QueryAndBuildEvent(ctx, &proto, identity, evTime, rsAPI, nil)
}

func buildMembershipEvent(
	ctx context.Context,
	targetUserID, reason string, profileAPI userapi.ClientUserAPI,
	device *userapi.Device,
	membership, roomID string, isDirect bool,
	cfg *config.ClientAPI, evTime time.Time,
	rsAPI roomserverAPI.ClientRoomserverAPI, asAPI appserviceAPI.AppServiceInternalAPI,
) (*types.HeaderedEvent, error) {
	profile, err := loadProfile(ctx, targetUserID, cfg, profileAPI, asAPI)
	if err != nil {
		return nil, err
	}

	userID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		return nil, err
	}
	validRoomID, err := spec.NewRoomID(roomID)
	if err != nil {
		return nil, err
	}
	senderID, err := rsAPI.QuerySenderIDForUser(ctx, *validRoomID, *userID)
	if err != nil {
		return nil, err
	}

	targetID, err := spec.NewUserID(targetUserID, true)
	if err != nil {
		return nil, err
	}
	targetSenderID, err := rsAPI.QuerySenderIDForUser(ctx, *validRoomID, *targetID)
	if err != nil {
		return nil, err
	}

	identity, err := rsAPI.SigningIdentityFor(ctx, *validRoomID, *userID)
	if err != nil {
		return nil, err
	}

	return buildMembershipEventDirect(ctx, targetSenderID, reason, profile.DisplayName, profile.AvatarURL,
		senderID, device.UserDomain(), membership, roomID, isDirect, identity.KeyID, identity.PrivateKey, evTime, rsAPI)
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
			JSON: spec.InvalidParam(err.Error()),
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
	switch e := err.(type) {
	case nil:
	case threepid.ErrMissingParameter:
		util.GetLogger(req.Context()).WithError(err).Error("threepid.CheckAndProcessInvite failed")
		return inviteStored, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON(err.Error()),
		}
	case threepid.ErrNotTrusted:
		util.GetLogger(req.Context()).WithError(err).Error("threepid.CheckAndProcessInvite failed")
		return inviteStored, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.NotTrusted(body.IDServer),
		}
	case eventutil.ErrRoomNoExists:
		util.GetLogger(req.Context()).WithError(err).Error("threepid.CheckAndProcessInvite failed")
		return inviteStored, &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound(err.Error()),
		}
	case gomatrixserverlib.BadJSONError:
		util.GetLogger(req.Context()).WithError(err).Error("threepid.CheckAndProcessInvite failed")
		return inviteStored, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON(e.Error()),
		}
	default:
		util.GetLogger(req.Context()).WithError(err).Error("threepid.CheckAndProcessInvite failed")
		return inviteStored, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	return
}

func checkMemberInRoom(ctx context.Context, rsAPI roomserverAPI.ClientRoomserverAPI, userID spec.UserID, roomID string) *util.JSONResponse {
	var membershipRes roomserverAPI.QueryMembershipForUserResponse
	err := rsAPI.QueryMembershipForUser(ctx, &roomserverAPI.QueryMembershipForUserRequest{
		RoomID: roomID,
		UserID: userID,
	}, &membershipRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("QueryMembershipForUser: could not query membership for user")
		return &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	if !membershipRes.IsInRoom {
		return &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("user does not belong to room"),
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

	deviceUserID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("You don't have permission to kick this user, bad userID"),
		}
	}

	var membershipRes roomserverAPI.QueryMembershipForUserResponse
	membershipReq := roomserverAPI.QueryMembershipForUserRequest{
		RoomID: roomID,
		UserID: *deviceUserID,
	}
	err = rsAPI.QueryMembershipForUser(ctx, &membershipReq, &membershipRes)
	if err != nil {
		logger.WithError(err).Error("QueryMembershipForUser: could not query membership for user")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	if !membershipRes.RoomExists {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("room does not exist"),
		}
	}
	if membershipRes.IsInRoom {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.Unknown(fmt.Sprintf("User %s is in room %s", device.UserID, roomID)),
		}
	}

	request := roomserverAPI.PerformForgetRequest{
		RoomID: roomID,
		UserID: device.UserID,
	}
	response := roomserverAPI.PerformForgetResponse{}
	if err := rsAPI.PerformForget(ctx, &request, &response); err != nil {
		logger.WithError(err).Error("PerformForget: unable to forget room")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

func getPowerlevels(req *http.Request, rsAPI roomserverAPI.ClientRoomserverAPI, roomID string) (*gomatrixserverlib.PowerLevelContent, *util.JSONResponse) {
	plEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: spec.MRoomPowerLevels,
		StateKey:  "",
	})
	if plEvent == nil {
		return nil, &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("You don't have permission to perform this action, no power_levels event in this room."),
		}
	}
	pl, err := plEvent.PowerLevels()
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("You don't have permission to perform this action, the power_levels event for this room is malformed so auth checks cannot be performed."),
		}
	}
	return pl, nil
}
