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
	"sync"
	"time"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
)

type PDUInfo struct {
	Version   string          `json:"room_version"`
	ViaServer string          `json:"via_server,omitempty"`
	PDU       json.RawMessage `json:"pdu"`
}

type sendPDUsRequest struct {
	PDUs []PDUInfo `json:"pdus"`
}

// SendPDUs implements /sendPDUs
// nolint:gocyclo
func SendPDUs(
	req *http.Request, device *api.Device,
	cfg *config.ClientAPI,
	profileAPI api.ClientUserAPI, rsAPI roomserverAPI.ClientRoomserverAPI,
	asAPI appserviceAPI.AppServiceInternalAPI,
	txnID *string,
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

	// create a mutex for the specific user in the specific room
	// this avoids a situation where events that are received in quick succession are sent to the roomserver in a jumbled order
	// TODO: cryptoIDs - where to get roomID from?
	mutex, _ := userRoomSendMutexes.LoadOrStore("roomID"+userID.String(), &sync.Mutex{})
	mutex.(*sync.Mutex).Lock()
	defer mutex.(*sync.Mutex).Unlock()

	inputs := make([]roomserverAPI.InputRoomEvent, 0, len(pdus.PDUs))
	for _, event := range pdus.PDUs {
		// TODO: cryptoIDs - event hash check?
		verImpl, err := gomatrixserverlib.GetRoomVersion(gomatrixserverlib.RoomVersion(event.Version))
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

		pdu, err := verImpl.NewEventFromUntrustedJSON(event.PDU)
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{Err: err.Error()},
			}
		}

		util.GetLogger(req.Context()).Infof("Processing %s event (%s): %s", pdu.Type(), pdu.EventID(), pdu.JSON())

		// Check that the event is signed by the server sending the request.
		redacted, err := verImpl.RedactEventJSON(pdu.JSON())
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("RedactEventJSON failed")
			continue
		}

		verifier := gomatrixserverlib.JSONVerifierSelf{}
		verifyRequests := []gomatrixserverlib.VerifyJSONRequest{{
			ServerName:           spec.ServerName(pdu.SenderID()),
			Message:              redacted,
			AtTS:                 pdu.OriginServerTS(),
			ValidityCheckingFunc: gomatrixserverlib.StrictValiditySignatureCheck,
		}}
		verifyResults, err := verifier.VerifyJSONs(req.Context(), verifyRequests)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("keys.VerifyJSONs failed")
			continue
		}
		if verifyResults[0].Error != nil {
			util.GetLogger(req.Context()).WithError(verifyResults[0].Error).Error("Signature check failed: ")
			continue
		}

		switch pdu.Type() {
		case spec.MRoomCreate:
		case spec.MRoomMember:
			var membership gomatrixserverlib.MemberContent
			err = json.Unmarshal(pdu.Content(), &membership)
			switch {
			case err != nil:
				util.GetLogger(req.Context()).Errorf("m.room.member event (%s) content invalid: %v", pdu.EventID(), pdu.Content())
				continue
			case membership.Membership == spec.Join:
				deviceUserID, err := spec.NewUserID(device.UserID, true)
				if err != nil {
					return util.JSONResponse{
						Code: http.StatusForbidden,
						JSON: spec.Forbidden("userID doesn't have power level to change visibility"),
					}
				}
				if !cfg.Matrix.IsLocalServerName(pdu.RoomID().Domain()) {
					queryReq := roomserverAPI.QueryMembershipForUserRequest{
						RoomID: pdu.RoomID().String(),
						UserID: *deviceUserID,
					}
					var queryRes roomserverAPI.QueryMembershipForUserResponse
					if err := rsAPI.QueryMembershipForUser(req.Context(), &queryReq, &queryRes); err != nil {
						util.GetLogger(req.Context()).WithError(err).Error("rsAPI.QueryMembershipsForRoom failed")
						return util.JSONResponse{
							Code: http.StatusInternalServerError,
							JSON: spec.InternalServerError{},
						}
					}
					if !queryRes.IsInRoom {
						// This is a join event to a remote room
						// TODO: cryptoIDs - figure out how to obtain unsigned contents for outstanding federated invites
						joinReq := roomserverAPI.PerformJoinRequestCryptoIDs{
							RoomID:      pdu.RoomID().String(),
							UserID:      device.UserID,
							IsGuest:     device.AccountType == api.AccountTypeGuest,
							ServerNames: []spec.ServerName{spec.ServerName(event.ViaServer)},
							JoinEvent:   pdu,
						}
						err := rsAPI.PerformSendJoinCryptoIDs(req.Context(), &joinReq)
						if err != nil {
							util.GetLogger(req.Context()).Errorf("Failed processing %s event (%s): %v", pdu.Type(), pdu.EventID(), err)
						}
						continue // NOTE: don't send this event on to the roomserver
					}
				}
			case membership.Membership == spec.Invite:
				stateKey := pdu.StateKey()
				if stateKey == nil {
					return util.JSONResponse{
						Code: http.StatusForbidden,
						JSON: spec.Forbidden("invalid state_key for membership event"),
					}
				}
				invitedUserID, err := rsAPI.QueryUserIDForSender(req.Context(), pdu.RoomID(), spec.SenderID(*stateKey))
				if err != nil || invitedUserID == nil {
					return util.JSONResponse{
						Code: http.StatusNotFound,
						JSON: spec.NotFound("cannot find userID for invite event"),
					}
				}
				if !cfg.Matrix.IsLocalServerName(spec.ServerName(invitedUserID.Domain())) {
					inviteReq := roomserverAPI.PerformInviteRequestCryptoIDs{
						RoomID:      pdu.RoomID().String(),
						UserID:      *invitedUserID,
						InviteEvent: pdu,
					}
					err := rsAPI.PerformSendInviteCryptoIDs(req.Context(), &inviteReq)
					if err != nil {
						util.GetLogger(req.Context()).Errorf("Failed processing %s event (%s): %v", pdu.Type(), pdu.EventID(), err)
					}
					continue // NOTE: don't send this event on to the roomserver
				}
			}
		}

		// TODO: cryptoIDs - does it matter which order these are added?
		// yes - if the events are for room creation.
		// Could make this a client requirement? ie. events are processed based on the order they appear
		// We need to check event validity after processing each event.
		// ie. what if the client changes power levels that disallow further events they sent?
		// We should be doing this already as part of `SendInputRoomEvents`, but how should we pass this
		// failure back to the client?

		var transactionID *roomserverAPI.TransactionID
		if txnID != nil {
			transactionID = &roomserverAPI.TransactionID{
				SessionID: device.SessionID, TransactionID: *txnID,
			}
		}

		inputs = append(inputs, roomserverAPI.InputRoomEvent{
			Kind:   roomserverAPI.KindNew,
			Event:  &types.HeaderedEvent{PDU: pdu},
			Origin: userID.Domain(),
			// TODO: cryptoIDs - what to do with this field?
			// should probably generate this based on the event type being sent?
			//SendAsServer: roomserverAPI.DoNotSendToOtherServers,
			TransactionID: transactionID,
		})
	}

	startedSubmittingEvents := time.Now()
	// send the events to the roomserver
	if err := roomserverAPI.SendInputRoomEvents(req.Context(), rsAPI, userID.Domain(), inputs, false); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("roomserverAPI.SendInputRoomEvents failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{Err: err.Error()},
		}
	}
	timeToSubmitEvents := time.Since(startedSubmittingEvents)
	sendEventDuration.With(prometheus.Labels{"action": "submit"}).Observe(float64(timeToSubmitEvents.Milliseconds()))

	// Add response to transactionsCache
	if txnID != nil {
		// TODO : cryptoIDs - fix this
		//res := util.JSONResponse{
		//	Code: http.StatusOK,
		//	JSON: sendEventResponse{e.EventID()},
		//}
		//txnCache.AddTransaction(device.AccessToken, *txnID, req.URL, &res)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
	}
}
