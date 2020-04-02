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
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// Invite implements /_matrix/federation/v2/invite/{roomID}/{eventID}
func Invite(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	roomID string,
	eventID string,
	cfg *config.Dendrite,
	producer *producers.RoomserverProducer,
	keys gomatrixserverlib.KeyRing,
) util.JSONResponse {
	inviteReq := gomatrixserverlib.InviteV2Request{}
	if err := json.Unmarshal(request.Content(), &inviteReq); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotJSON("The request body could not be decoded into an invite request. " + err.Error()),
		}
	}
	event := inviteReq.Event()

	// Check that the room ID is correct.
	if event.RoomID() != roomID {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The room ID in the request path must match the room ID in the invite event JSON"),
		}
	}

	// Check that the event ID is correct.
	if event.EventID() != eventID {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The event ID in the request path must match the event ID in the invite event JSON"),
		}
	}

	// Check that the event is signed by the server sending the request.
	redacted := event.Redact()
	verifyRequests := []gomatrixserverlib.VerifyJSONRequest{{
		ServerName: event.Origin(),
		Message:    redacted.JSON(),
		AtTS:       event.OriginServerTS(),
	}}
	verifyResults, err := keys.VerifyJSONs(httpReq.Context(), verifyRequests)
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("keys.VerifyJSONs failed")
		return jsonerror.InternalServerError()
	}
	if verifyResults[0].Error != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("The invite must be signed by the server it originated on"),
		}
	}

	// Sign the event so that other servers will know that we have received the invite.
	signedEvent := event.Sign(
		string(cfg.Matrix.ServerName), cfg.Matrix.KeyID, cfg.Matrix.PrivateKey,
	)

	// Add the invite event to the roomserver.
	if err = producer.SendInvite(httpReq.Context(), signedEvent.Headered(inviteReq.RoomVersion())); err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("producer.SendInvite failed")
		return jsonerror.InternalServerError()
	}

	// Return the signed event to the originating server, it should then tell
	// the other servers in the room that we have been invited.
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: gomatrixserverlib.RespInvite{Event: signedEvent},
	}
}
