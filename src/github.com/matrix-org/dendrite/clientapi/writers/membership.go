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

package writers

import (
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/events"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/threepid"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/util"
)

var errMissingUserID = errors.New("'user_id' must be supplied")

// SendMembership implements PUT /rooms/{roomID}/(join|kick|ban|unban|leave|invite)
// by building a m.room.member event then sending it to the room server
func SendMembership(
	req *http.Request, accountDB *accounts.Database, device *authtypes.Device,
	roomID string, membership string, cfg config.Dendrite,
	queryAPI api.RoomserverQueryAPI, producer *producers.RoomserverProducer,
) util.JSONResponse {
	var body threepid.MembershipRequest
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	inviteStored, err := threepid.CheckAndProcessInvite(
		device, &body, cfg, queryAPI, accountDB, producer, membership, roomID,
	)
	if err == threepid.ErrMissingParameter {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON(err.Error()),
		}
	} else if err == threepid.ErrNotTrusted {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.NotTrusted(body.IDServer),
		}
	} else if err == events.ErrRoomNoExists {
		return util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound(err.Error()),
		}
	} else if err != nil {
		return httputil.LogThenError(req, err)
	}

	// If an invite has been stored on an identity server, it means that a
	// m.room.third_party_invite event has been emitted and that we shouldn't
	// emit a m.room.member one.
	if inviteStored {
		return util.JSONResponse{
			Code: 200,
			JSON: struct{}{},
		}
	}

	event, err := buildMembershipEvent(
		body, accountDB, device, membership, roomID, cfg, queryAPI,
	)
	if err == errMissingUserID {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON(err.Error()),
		}
	} else if err == events.ErrRoomNoExists {
		return util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound(err.Error()),
		}
	} else if err != nil {
		return httputil.LogThenError(req, err)
	}

	if err := producer.SendEvents([]gomatrixserverlib.Event{*event}, cfg.Matrix.ServerName); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}

func buildMembershipEvent(
	body threepid.MembershipRequest, accountDB *accounts.Database,
	device *authtypes.Device, membership string, roomID string, cfg config.Dendrite,
	queryAPI api.RoomserverQueryAPI,
) (*gomatrixserverlib.Event, error) {
	stateKey, reason, err := getMembershipStateKey(body, device, membership)
	if err != nil {
		return nil, err
	}

	profile, err := loadProfile(stateKey, cfg, accountDB)
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
		membership = "leave"
	}

	content := common.MemberContent{
		Membership:  membership,
		DisplayName: profile.DisplayName,
		AvatarURL:   profile.AvatarURL,
		Reason:      reason,
	}

	if err = builder.SetContent(content); err != nil {
		return nil, err
	}

	return events.BuildEvent(&builder, cfg, queryAPI, nil)
}

// loadProfile lookups the profile of a given user from the database and returns
// it if the user is local to this server, or returns an empty profile if not.
// Returns an error if the retrieval failed or if the first parameter isn't a
// valid Matrix ID.
func loadProfile(userID string, cfg config.Dendrite, accountDB *accounts.Database) (*authtypes.Profile, error) {
	localpart, serverName, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return nil, err
	}

	var profile *authtypes.Profile
	if serverName == cfg.Matrix.ServerName {
		profile, err = accountDB.GetProfileByLocalpart(localpart)
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
	if membership == "ban" || membership == "unban" || membership == "kick" || membership == "invite" {
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
