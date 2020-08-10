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
	"errors"
	"net/http"
	"time"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/threepid"
	currentstateAPI "github.com/matrix-org/dendrite/currentstateserver/api"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/util"
)

var errMissingUserID = errors.New("'user_id' must be supplied")

func SendBan(
	req *http.Request, accountDB accounts.Database, device *userapi.Device,
	roomID string, cfg *config.ClientAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI, asAPI appserviceAPI.AppServiceQueryAPI,
) util.JSONResponse {
	body, evTime, roomVer, reqErr := extractRequestData(req, roomID, rsAPI)
	if reqErr != nil {
		return *reqErr
	}
	return sendMembership(req.Context(), accountDB, device, roomID, "ban", body.Reason, cfg, body.UserID, evTime, roomVer, rsAPI, asAPI)
}

func sendMembership(ctx context.Context, accountDB accounts.Database, device *userapi.Device,
	roomID, membership, reason string, cfg *config.ClientAPI, targetUserID string, evTime time.Time,
	roomVer gomatrixserverlib.RoomVersion,
	rsAPI roomserverAPI.RoomserverInternalAPI, asAPI appserviceAPI.AppServiceQueryAPI) util.JSONResponse {

	event, err := buildMembershipEvent(
		ctx, targetUserID, reason, accountDB, device, membership,
		roomID, false, cfg, evTime, rsAPI, asAPI,
	)
	if err == errMissingUserID {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(err.Error()),
		}
	} else if err == eventutil.ErrRoomNoExists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound(err.Error()),
		}
	} else if err != nil {
		util.GetLogger(ctx).WithError(err).Error("buildMembershipEvent failed")
		return jsonerror.InternalServerError()
	}

	_, err = roomserverAPI.SendEvents(
		ctx, rsAPI,
		[]gomatrixserverlib.HeaderedEvent{event.Event.Headered(roomVer)},
		cfg.Matrix.ServerName,
		nil,
	)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("SendEvents failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

func SendKick(
	req *http.Request, accountDB accounts.Database, device *userapi.Device,
	roomID string, cfg *config.ClientAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI, asAPI appserviceAPI.AppServiceQueryAPI,
	stateAPI currentstateAPI.CurrentStateInternalAPI,
) util.JSONResponse {
	body, evTime, roomVer, reqErr := extractRequestData(req, roomID, rsAPI)
	if reqErr != nil {
		return *reqErr
	}
	if body.UserID == "" {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("missing user_id"),
		}
	}

	errRes := checkMemberInRoom(req.Context(), stateAPI, device.UserID, roomID)
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
	// kick is only valid if the user is not currently banned or left (that is, they are joined or invited)
	if queryRes.Membership != "join" && queryRes.Membership != "invite" {
		return util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Unknown("cannot /kick banned or left users"),
		}
	}
	// TODO: should we be using SendLeave instead?
	return sendMembership(req.Context(), accountDB, device, roomID, "leave", body.Reason, cfg, body.UserID, evTime, roomVer, rsAPI, asAPI)
}

func SendUnban(
	req *http.Request, accountDB accounts.Database, device *userapi.Device,
	roomID string, cfg *config.ClientAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI, asAPI appserviceAPI.AppServiceQueryAPI,
) util.JSONResponse {
	body, evTime, roomVer, reqErr := extractRequestData(req, roomID, rsAPI)
	if reqErr != nil {
		return *reqErr
	}
	if body.UserID == "" {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("missing user_id"),
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
	// unban is only valid if the user is currently banned
	if queryRes.Membership != "ban" {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown("can only /unban users that are banned"),
		}
	}
	// TODO: should we be using SendLeave instead?
	return sendMembership(req.Context(), accountDB, device, roomID, "leave", body.Reason, cfg, body.UserID, evTime, roomVer, rsAPI, asAPI)
}

func SendInvite(
	req *http.Request, accountDB accounts.Database, device *userapi.Device,
	roomID string, cfg *config.ClientAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI, asAPI appserviceAPI.AppServiceQueryAPI,
) util.JSONResponse {
	body, evTime, roomVer, reqErr := extractRequestData(req, roomID, rsAPI)
	if reqErr != nil {
		return *reqErr
	}

	inviteStored, jsonErrResp := checkAndProcessThreepid(
		req, device, body, cfg, rsAPI, accountDB, roomID, evTime,
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

	event, err := buildMembershipEvent(
		req.Context(), body.UserID, body.Reason, accountDB, device, "invite",
		roomID, false, cfg, evTime, rsAPI, asAPI,
	)
	if err == errMissingUserID {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(err.Error()),
		}
	} else if err == eventutil.ErrRoomNoExists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound(err.Error()),
		}
	} else if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("buildMembershipEvent failed")
		return jsonerror.InternalServerError()
	}

	perr := roomserverAPI.SendInvite(
		req.Context(), rsAPI,
		event.Event.Headered(roomVer),
		nil, // ask the roomserver to draw up invite room state for us
		cfg.Matrix.ServerName,
		nil,
	)
	if perr != nil {
		util.GetLogger(req.Context()).WithError(perr).Error("producer.SendInvite failed")
		return perr.JSONResponse()
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

func buildMembershipEvent(
	ctx context.Context,
	targetUserID, reason string, accountDB accounts.Database,
	device *userapi.Device,
	membership, roomID string, isDirect bool,
	cfg *config.ClientAPI, evTime time.Time,
	rsAPI roomserverAPI.RoomserverInternalAPI, asAPI appserviceAPI.AppServiceQueryAPI,
) (*gomatrixserverlib.HeaderedEvent, error) {
	profile, err := loadProfile(ctx, targetUserID, cfg, accountDB, asAPI)
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

	return eventutil.BuildEvent(ctx, &builder, cfg.Matrix, evTime, rsAPI, nil)
}

// loadProfile lookups the profile of a given user from the database and returns
// it if the user is local to this server, or returns an empty profile if not.
// Returns an error if the retrieval failed or if the first parameter isn't a
// valid Matrix ID.
func loadProfile(
	ctx context.Context,
	userID string,
	cfg *config.ClientAPI,
	accountDB accounts.Database,
	asAPI appserviceAPI.AppServiceQueryAPI,
) (*authtypes.Profile, error) {
	_, serverName, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return nil, err
	}

	var profile *authtypes.Profile
	if serverName == cfg.Matrix.ServerName {
		profile, err = appserviceAPI.RetrieveUserProfile(ctx, userID, asAPI, accountDB)
	} else {
		profile = &authtypes.Profile{}
	}

	return profile, err
}

func extractRequestData(req *http.Request, roomID string, rsAPI api.RoomserverInternalAPI) (
	body *threepid.MembershipRequest, evTime time.Time, roomVer gomatrixserverlib.RoomVersion, resErr *util.JSONResponse,
) {
	verReq := api.QueryRoomVersionForRoomRequest{RoomID: roomID}
	verRes := api.QueryRoomVersionForRoomResponse{}
	if err := rsAPI.QueryRoomVersionForRoom(req.Context(), &verReq, &verRes); err != nil {
		resErr = &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.UnsupportedRoomVersion(err.Error()),
		}
		return
	}
	roomVer = verRes.RoomVersion

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
	rsAPI roomserverAPI.RoomserverInternalAPI,
	accountDB accounts.Database,
	roomID string,
	evTime time.Time,
) (inviteStored bool, errRes *util.JSONResponse) {

	inviteStored, err := threepid.CheckAndProcessInvite(
		req.Context(), device, body, cfg, rsAPI, accountDB,
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

func checkMemberInRoom(ctx context.Context, stateAPI currentstateAPI.CurrentStateInternalAPI, userID, roomID string) *util.JSONResponse {
	tuple := gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomMember,
		StateKey:  userID,
	}
	var membershipRes currentstateAPI.QueryCurrentStateResponse
	err := stateAPI.QueryCurrentState(ctx, &currentstateAPI.QueryCurrentStateRequest{
		RoomID:      roomID,
		StateTuples: []gomatrixserverlib.StateKeyTuple{tuple},
	}, &membershipRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("QueryCurrentState: could not query membership for user")
		e := jsonerror.InternalServerError()
		return &e
	}
	ev, ok := membershipRes.StateEvents[tuple]
	if !ok {
		return &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("user does not belong to room"),
		}
	}
	membership, err := ev.Membership()
	if err != nil || membership != "join" {
		return &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("user does not belong to room"),
		}
	}
	return nil
}
