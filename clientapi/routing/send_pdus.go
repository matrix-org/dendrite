// Copyright 2023 The Matrix.org Foundation C.I.C.
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

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

type sendPDUsRequest struct {
	Version string            `json:"room_version"`
	PDUs    []json.RawMessage `json:"pdus"`
}

// SendPDUs implements /sendPDUs
func SendPDUs(
	req *http.Request, device *api.Device,
	cfg *config.ClientAPI,
	profileAPI api.ClientUserAPI, rsAPI roomserverAPI.ClientRoomserverAPI,
	asAPI appserviceAPI.AppServiceInternalAPI,
) util.JSONResponse {
	// TODO: cryptoIDs - should this include an "eventType"?
	// if it's a bulk send endpoint, I don't think that makes any sense since there are multiple event types
	// In that case, how do I know how to treat the events?
	// I could sort them all by roomID?
	// Then filter them down based on event type? (how do I collect groups of events such as for room creation?)
	// Possibly based on event hash tracking that I know were sent to the client?
	// For createRoom, I know what the possible list of events are, so try to find those and collect them to send to room creation.
	// Could also sort by depth... but that seems dangerous and depth may not be a field forever
	// Does it matter at all?
	// Can't I just forward all the events to the roomserver?
	// Do I need to do any specific processing on them?

	var pdus sendPDUsRequest
	resErr := httputil.UnmarshalJSONRequest(req, &pdus)
	if resErr != nil {
		return *resErr
	}

	userID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam(err.Error()),
		}
	}

	inputs := make([]roomserverAPI.InputRoomEvent, 0, len(pdus.PDUs))
	for _, event := range pdus.PDUs {
		// TODO: cryptoIDs - event hash check?
		verImpl, err := gomatrixserverlib.GetRoomVersion(gomatrixserverlib.RoomVersion(pdus.Version))
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{Err: err.Error()},
			}
		}
		//eventJSON, err := json.Marshal(event)
		//if err != nil {
		//	return util.JSONResponse{
		//		Code: http.StatusInternalServerError,
		//		JSON: spec.InternalServerError{Err: err.Error()},
		//	}
		//}
		// TODO: cryptoIDs - how should we be converting to a PDU here?
		// if the hash matches an event we sent to the client, then the JSON should be good.
		// But how do we know how to fill out if the event is redacted if we use the trustedJSON function?
		// Also - untrusted JSON seems better - except it strips off the unsigned field?
		// Also - gmsl events don't store the `hashes` field... problem?

		pdu, err := verImpl.NewEventFromUntrustedJSON(event)
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{Err: err.Error()},
			}
		}
		key, err := rsAPI.GetOrCreateUserRoomPrivateKey(req.Context(), *userID, pdu.RoomID())
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{Err: err.Error()},
			}
		}
		pdu = pdu.Sign(string(pdu.SenderID()), "ed25519:1", key)
		util.GetLogger(req.Context()).Infof("Processing %s event (%s)", pdu.Type(), pdu.EventID())
		switch pdu.Type() {
		case spec.MRoomCreate:
		}

		// TODO: cryptoIDs - does it matter which order these are added?
		// yes - if the events are for room creation.
		// Could make this a client requirement? ie. events are processed based on the order they appear
		// We need to check event validity after processing each event.
		// ie. what if the client changes power levels that disallow further events they sent?
		// We should be doing this already as part of `SendInputRoomEvents`, but how should we pass this
		// failure back to the client?
		inputs = append(inputs, roomserverAPI.InputRoomEvent{
			Kind:         roomserverAPI.KindNew,
			Event:        &types.HeaderedEvent{PDU: pdu},
			Origin:       userID.Domain(),
			SendAsServer: roomserverAPI.DoNotSendToOtherServers,
		})
	}

	// send the events to the roomserver
	if err := roomserverAPI.SendInputRoomEvents(req.Context(), rsAPI, userID.Domain(), inputs, false); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("roomserverAPI.SendInputRoomEvents failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{Err: err.Error()},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
	}
}
