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
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// Invite implements /_matrix/federation/v1/invite/{roomID}/{eventID}
func Invite(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	roomID string,
	eventID string,
	cfg config.Dendrite,
	producer *producers.RoomserverProducer,
	keys gomatrixserverlib.KeyRing,
) util.JSONResponse {

	// Decode the event JSON from the request.
	var event gomatrixserverlib.Event
	if err := json.Unmarshal(request.Content(), &event); err != nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.NotJSON("The request body could not be decoded into valid JSON. " + err.Error()),
		}
	}

	// Check that the room ID is correct.
	if event.RoomID() != roomID {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("The room ID in the request path must match the room ID in the invite event JSON"),
		}
	}

	// Check that the event ID is correct.
	if event.EventID() != eventID {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("The event ID in the request path must match the event ID in the invite event JSON"),
		}
	}

	// Check that the event is from the server sending the request.
	if event.Origin() != request.Origin() {
		return util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("The invite must be sent by the server it originated on"),
		}
	}

	// Check that the event is signed by the server sending the request.
	verifyRequests := []gomatrixserverlib.VerifyJSONRequest{{
		ServerName: event.Origin(),
		Message:    event.Redact().JSON(),
		AtTS:       event.OriginServerTS(),
	}}
	verifyResults, err := keys.VerifyJSONs(httpReq.Context(), verifyRequests)
	if err != nil {
		return httputil.LogThenError(httpReq, err)
	}
	if verifyResults[0].Error != nil {
		return util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("The invite must be signed by the server it originated on"),
		}
	}

	// Sign the event so that other servers will know that we have received the invite.
	signedEvent := event.Sign(
		string(cfg.Matrix.ServerName), cfg.Matrix.KeyID, cfg.Matrix.PrivateKey,
	)

	// Add the invite event to the roomserver.
	if err = producer.SendInvite(httpReq.Context(), signedEvent); err != nil {
		return httputil.LogThenError(httpReq, err)
	}

	// Return the signed event to the originating server, it should then tell
	// the other servers in the room that we have been invited.
	return util.JSONResponse{
		Code: 200,
		JSON: gomatrixserverlib.RespInvite{Event: signedEvent},
	}
}
