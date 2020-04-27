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
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/threepid"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/util"
)

var errMissingUserID = errors.New("'user_id' must be supplied")

// SendMembership implements PUT /rooms/{roomID}/(join|kick|ban|unban|leave|invite)
// by building a m.room.member event then sending it to the room server
// TODO: Can we improve the cyclo count here? Separate code paths for invites?
// nolint:gocyclo
func SendMembership(
	req *http.Request, accountDB accounts.Database, device *authtypes.Device,
	roomID string, membership string, cfg *config.Dendrite,
	queryAPI roomserverAPI.RoomserverQueryAPI, asAPI appserviceAPI.AppServiceQueryAPI,
	producer *producers.RoomserverProducer,
) util.JSONResponse {
	verReq := api.QueryRoomVersionForRoomRequest{RoomID: roomID}
	verRes := api.QueryRoomVersionForRoomResponse{}
	if err := queryAPI.QueryRoomVersionForRoom(req.Context(), &verReq, &verRes); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.UnsupportedRoomVersion(err.Error()),
		}
	}

	var body threepid.MembershipRequest
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	evTime, err := httputil.ParseTSParam(req)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue(err.Error()),
		}
	}

	inviteStored, jsonErrResp := checkAndProcessThreepid(
		req, device, &body, cfg, queryAPI, accountDB, producer,
		membership, roomID, evTime,
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
		req.Context(), body, accountDB, device, membership, roomID, cfg, evTime, queryAPI, asAPI,
	)
	if err == errMissingUserID {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(err.Error()),
		}
	} else if err == common.ErrRoomNoExists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound(err.Error()),
		}
	} else if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("buildMembershipEvent failed")
		return jsonerror.InternalServerError()
	}

	var returnData interface{} = struct{}{}

	switch membership {
	case gomatrixserverlib.Invite:
		// Invites need to be handled specially
		err = producer.SendInvite(
			req.Context(),
			event.Headered(verRes.RoomVersion),
			nil, // ask the roomserver to draw up invite room state for us
			cfg.Matrix.ServerName,
			nil,
		)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("producer.SendInvite failed")
			return jsonerror.InternalServerError()
		}
	case gomatrixserverlib.Join:
		// The join membership requires the room id to be sent in the response
		returnData = struct {
			RoomID string `json:"room_id"`
		}{roomID}
	default:
		_, err = producer.SendEvents(
			req.Context(),
			[]gomatrixserverlib.HeaderedEvent{event.Headered(verRes.RoomVersion)},
			cfg.Matrix.ServerName,
			nil,
		)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("producer.SendEvents failed")
			return jsonerror.InternalServerError()
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: returnData,
	}
}

func buildMembershipEvent(
	ctx context.Context,
	body threepid.MembershipRequest, accountDB accounts.Database,
	device *authtypes.Device,
	membership, roomID string,
	cfg *config.Dendrite, evTime time.Time,
	queryAPI roomserverAPI.RoomserverQueryAPI, asAPI appserviceAPI.AppServiceQueryAPI,
) (*gomatrixserverlib.Event, error) {
	stateKey, reason, err := getMembershipStateKey(body, device, membership)
	if err != nil {
		return nil, err
	}

	profile, err := loadProfile(ctx, stateKey, cfg, accountDB, asAPI)
	if err != nil {
		return nil, err
	}

	builder := gomatrixserverlib.EventBuilder{
		Sender:   device.UserID,
		RoomID:   roomID,
		Type:     "m.room.member",
		StateKey: &stateKey,
	}

	// "unban" or "kick" isn't a valid membership value, change it to "leave"
	if membership == "unban" || membership == "kick" {
		membership = gomatrixserverlib.Leave
	}

	content := gomatrixserverlib.MemberContent{
		Membership:  membership,
		DisplayName: profile.DisplayName,
		AvatarURL:   profile.AvatarURL,
		Reason:      reason,
	}

	if err = builder.SetContent(content); err != nil {
		return nil, err
	}

	return common.BuildEvent(ctx, &builder, cfg, evTime, queryAPI, nil)
}

// loadProfile lookups the profile of a given user from the database and returns
// it if the user is local to this server, or returns an empty profile if not.
// Returns an error if the retrieval failed or if the first parameter isn't a
// valid Matrix ID.
func loadProfile(
	ctx context.Context,
	userID string,
	cfg *config.Dendrite,
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

// getMembershipStateKey extracts the target user ID of a membership change.
// For "join" and "leave" this will be the ID of the user making the change.
// For "ban", "unban", "kick" and "invite" the target user ID will be in the JSON request body.
// In the latter case, if there was an issue retrieving the user ID from the request body,
// returns a JSONResponse with a corresponding error code and message.
func getMembershipStateKey(
	body threepid.MembershipRequest, device *authtypes.Device, membership string,
) (stateKey string, reason string, err error) {
	if membership == gomatrixserverlib.Ban || membership == "unban" || membership == "kick" || membership == gomatrixserverlib.Invite {
		// If we're in this case, the state key is contained in the request body,
		// possibly along with a reason (for "kick" and "ban") so we need to parse
		// it
		if body.UserID == "" {
			err = errMissingUserID
			return
		}

		stateKey = body.UserID
		reason = body.Reason
	} else {
		stateKey = device.UserID
	}

	return
}

func checkAndProcessThreepid(
	req *http.Request,
	device *authtypes.Device,
	body *threepid.MembershipRequest,
	cfg *config.Dendrite,
	queryAPI roomserverAPI.RoomserverQueryAPI,
	accountDB accounts.Database,
	producer *producers.RoomserverProducer,
	membership, roomID string,
	evTime time.Time,
) (inviteStored bool, errRes *util.JSONResponse) {

	inviteStored, err := threepid.CheckAndProcessInvite(
		req.Context(), device, body, cfg, queryAPI, accountDB, producer,
		membership, roomID, evTime,
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
	} else if err == common.ErrRoomNoExists {
		return inviteStored, &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound(err.Error()),
		}
	} else if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("threepid.CheckAndProcessInvite failed")
		er := jsonerror.InternalServerError()
		return inviteStored, &er
	}
	return
}
