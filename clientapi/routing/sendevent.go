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
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/internal/transactions"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

// http://matrix.org/docs/spec/client_server/r0.2.0.html#put-matrix-client-r0-rooms-roomid-send-eventtype-txnid
// http://matrix.org/docs/spec/client_server/r0.2.0.html#put-matrix-client-r0-rooms-roomid-state-eventtype-statekey
type sendEventResponse struct {
	EventID string `json:"event_id"`
}

var (
	userRoomSendMutexes sync.Map // (roomID+userID) -> mutex. mutexes to ensure correct ordering of sendEvents
)

var sendEventDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: "dendrite",
		Subsystem: "clientapi",
		Name:      "sendevent_duration_millis",
		Help:      "How long it takes to build and submit a new event from the client API to the roomserver",
		Buckets: []float64{ // milliseconds
			5, 10, 25, 50, 75, 100, 250, 500,
			1000, 2000, 3000, 4000, 5000, 6000,
			7000, 8000, 9000, 10000, 15000, 20000,
		},
	},
	[]string{"action"},
)

// SendEvent implements:
//
//	/rooms/{roomID}/send/{eventType}
//	/rooms/{roomID}/send/{eventType}/{txnID}
//	/rooms/{roomID}/state/{eventType}/{stateKey}
func SendEvent(
	req *http.Request,
	device *userapi.Device,
	roomID, eventType string, txnID, stateKey *string,
	cfg *config.ClientAPI,
	rsAPI api.ClientRoomserverAPI,
	txnCache *transactions.Cache,
) util.JSONResponse {
	roomVersion, err := rsAPI.QueryRoomVersionForRoom(req.Context(), roomID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.UnsupportedRoomVersion(err.Error()),
		}
	}

	if txnID != nil {
		// Try to fetch response from transactionsCache
		if res, ok := txnCache.FetchTransaction(device.AccessToken, *txnID, req.URL); ok {
			return *res
		}
	}

	// create a mutex for the specific user in the specific room
	// this avoids a situation where events that are received in quick succession are sent to the roomserver in a jumbled order
	userID := device.UserID
	domain := device.UserDomain()
	mutex, _ := userRoomSendMutexes.LoadOrStore(roomID+userID, &sync.Mutex{})
	mutex.(*sync.Mutex).Lock()
	defer mutex.(*sync.Mutex).Unlock()

	var r map[string]interface{} // must be a JSON object
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}

	if stateKey != nil {
		// If the existing/new state content are equal, return the existing event_id, making the request idempotent.
		if resp := stateEqual(req.Context(), rsAPI, eventType, *stateKey, roomID, r); resp != nil {
			return *resp
		}
	}

	startedGeneratingEvent := time.Now()

	// If we're sending a membership update, make sure to strip the authorised
	// via key if it is present, otherwise other servers won't be able to auth
	// the event if the room is set to the "restricted" join rule.
	if eventType == spec.MRoomMember {
		delete(r, "join_authorised_via_users_server")
	}

	evTime, err := httputil.ParseTSParam(req)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam(err.Error()),
		}
	}

	e, resErr := generateSendEvent(req.Context(), r, device, roomID, eventType, stateKey, rsAPI, evTime)
	if resErr != nil {
		return *resErr
	}
	timeToGenerateEvent := time.Since(startedGeneratingEvent)

	// validate that the aliases exists
	if eventType == spec.MRoomCanonicalAlias && stateKey != nil && *stateKey == "" {
		aliasReq := api.AliasEvent{}
		if err = json.Unmarshal(e.Content(), &aliasReq); err != nil {
			return util.ErrorResponse(fmt.Errorf("unable to parse alias event: %w", err))
		}
		if !aliasReq.Valid() {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam("Request contains invalid aliases."),
			}
		}
		aliasRes := &api.GetAliasesForRoomIDResponse{}
		if err = rsAPI.GetAliasesForRoomID(req.Context(), &api.GetAliasesForRoomIDRequest{RoomID: roomID}, aliasRes); err != nil {
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
		var found int
		requestAliases := append(aliasReq.AltAliases, aliasReq.Alias)
		for _, alias := range aliasRes.Aliases {
			for _, altAlias := range requestAliases {
				if altAlias == alias {
					found++
				}
			}
		}
		// check that we found at least the same amount of existing aliases as are in the request
		if aliasReq.Alias != "" && found < len(requestAliases) {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadAlias("No matching alias found."),
			}
		}
	}

	var txnAndSessionID *api.TransactionID
	if txnID != nil {
		txnAndSessionID = &api.TransactionID{
			TransactionID: *txnID,
			SessionID:     device.SessionID,
		}
	}

	// pass the new event to the roomserver and receive the correct event ID
	// event ID in case of duplicate transaction is discarded
	startedSubmittingEvent := time.Now()
	if err := api.SendEvents(
		req.Context(), rsAPI,
		api.KindNew,
		[]*types.HeaderedEvent{
			&types.HeaderedEvent{PDU: e},
		},
		device.UserDomain(),
		domain,
		domain,
		txnAndSessionID,
		false,
	); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("SendEvents failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	timeToSubmitEvent := time.Since(startedSubmittingEvent)
	util.GetLogger(req.Context()).WithFields(logrus.Fields{
		"event_id":     e.EventID(),
		"room_id":      roomID,
		"room_version": roomVersion,
	}).Info("Sent event to roomserver")

	res := util.JSONResponse{
		Code: http.StatusOK,
		JSON: sendEventResponse{e.EventID()},
	}
	// Add response to transactionsCache
	if txnID != nil {
		txnCache.AddTransaction(device.AccessToken, *txnID, req.URL, &res)
	}

	// Take a note of how long it took to generate the event vs submit
	// it to the roomserver.
	sendEventDuration.With(prometheus.Labels{"action": "build"}).Observe(float64(timeToGenerateEvent.Milliseconds()))
	sendEventDuration.With(prometheus.Labels{"action": "submit"}).Observe(float64(timeToSubmitEvent.Milliseconds()))

	return res
}

// stateEqual compares the new and the existing state event content. If they are equal, returns a *util.JSONResponse
// with the existing event_id, making this an idempotent request.
func stateEqual(ctx context.Context, rsAPI api.ClientRoomserverAPI, eventType, stateKey, roomID string, newContent map[string]interface{}) *util.JSONResponse {
	stateRes := api.QueryCurrentStateResponse{}
	tuple := gomatrixserverlib.StateKeyTuple{
		EventType: eventType,
		StateKey:  stateKey,
	}
	err := rsAPI.QueryCurrentState(ctx, &api.QueryCurrentStateRequest{
		RoomID:      roomID,
		StateTuples: []gomatrixserverlib.StateKeyTuple{tuple},
	}, &stateRes)
	if err != nil {
		return nil
	}
	if existingEvent, ok := stateRes.StateEvents[tuple]; ok {
		var existingContent map[string]interface{}
		if err = json.Unmarshal(existingEvent.Content(), &existingContent); err != nil {
			return nil
		}
		if reflect.DeepEqual(existingContent, newContent) {
			return &util.JSONResponse{
				Code: http.StatusOK,
				JSON: sendEventResponse{existingEvent.EventID()},
			}
		}

	}
	return nil
}

func generateSendEvent(
	ctx context.Context,
	r map[string]interface{},
	device *userapi.Device,
	roomID, eventType string, stateKey *string,
	rsAPI api.ClientRoomserverAPI,
	evTime time.Time,
) (gomatrixserverlib.PDU, *util.JSONResponse) {
	// parse the incoming http request
	fullUserID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("Bad userID"),
		}
	}
	validRoomID, err := spec.NewRoomID(roomID)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("RoomID is invalid"),
		}
	}
	senderID, err := rsAPI.QuerySenderIDForUser(ctx, *validRoomID, *fullUserID)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound("Unable to find senderID for user"),
		}
	}

	// create the new event and set all the fields we can
	proto := gomatrixserverlib.ProtoEvent{
		SenderID: string(senderID),
		RoomID:   roomID,
		Type:     eventType,
		StateKey: stateKey,
	}
	err = proto.SetContent(r)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("proto.SetContent failed")
		return nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	identity, err := rsAPI.SigningIdentityFor(ctx, *validRoomID, *fullUserID)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	var queryRes api.QueryLatestEventsAndStateResponse
	e, err := eventutil.QueryAndBuildEvent(ctx, &proto, &identity, evTime, rsAPI, &queryRes)
	switch specificErr := err.(type) {
	case nil:
	case eventutil.ErrRoomNoExists:
		return nil, &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound("Room does not exist"),
		}
	case gomatrixserverlib.BadJSONError:
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON(specificErr.Error()),
		}
	case gomatrixserverlib.EventValidationError:
		if specificErr.Code == gomatrixserverlib.EventValidationTooLarge {
			return nil, &util.JSONResponse{
				Code: http.StatusRequestEntityTooLarge,
				JSON: spec.BadJSON(specificErr.Error()),
			}
		}
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON(specificErr.Error()),
		}
	default:
		util.GetLogger(ctx).WithError(err).Error("eventutil.BuildEvent failed")
		return nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	// check to see if this user can perform this operation
	stateEvents := make([]gomatrixserverlib.PDU, len(queryRes.StateEvents))
	for i := range queryRes.StateEvents {
		stateEvents[i] = queryRes.StateEvents[i].PDU
	}
	provider := gomatrixserverlib.NewAuthEvents(gomatrixserverlib.ToPDUs(stateEvents))
	if err = gomatrixserverlib.Allowed(e.PDU, &provider, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
		return rsAPI.QueryUserIDForSender(ctx, *validRoomID, senderID)
	}); err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden(err.Error()), // TODO: Is this error string comprehensible to the client?
		}
	}

	// User should not be able to send a tombstone event to the same room.
	if e.Type() == "m.room.tombstone" {
		content := make(map[string]interface{})
		if err = json.Unmarshal(e.Content(), &content); err != nil {
			util.GetLogger(ctx).WithError(err).Error("Cannot unmarshal the event content.")
			return nil, &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON("Cannot unmarshal the event content."),
			}
		}
		if content["replacement_room"] == e.RoomID() {
			return nil, &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam("Cannot send tombstone event that points to the same room."),
			}
		}
	}

	return e.PDU, nil
}
