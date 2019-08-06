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
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/common/transactions"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// http://matrix.org/docs/spec/client_server/r0.2.0.html#put-matrix-client-r0-rooms-roomid-send-eventtype-txnid
// http://matrix.org/docs/spec/client_server/r0.2.0.html#put-matrix-client-r0-rooms-roomid-state-eventtype-statekey
type sendEventResponse struct {
	EventID string `json:"event_id"`
}

// SendEvent implements:
//   /rooms/{roomID}/send/{eventType}
//   /rooms/{roomID}/send/{eventType}/{txnID}
//   /rooms/{roomID}/state/{eventType}/{stateKey}
func SendEvent(
	req *http.Request,
	device *authtypes.Device,
	roomID, eventType string, txnID, stateKey *string,
	cfg config.Dendrite,
	queryAPI api.RoomserverQueryAPI,
	producer *producers.RoomserverProducer,
	txnCache *transactions.Cache,
) util.JSONResponse {
	if txnID != nil {
		// Try to fetch response from transactionsCache
		if res, ok := txnCache.FetchTransaction(device.AccessToken, *txnID); ok {
			return *res
		}
	}

	e, resErr := generateSendEvent(req, device, roomID, eventType, stateKey, cfg, queryAPI)
	if resErr != nil {
		return *resErr
	}

	var txnAndDeviceID *api.TransactionID
	if txnID != nil {
		txnAndDeviceID = &api.TransactionID{
			TransactionID: *txnID,
			DeviceID:      device.ID,
		}
	}

	// pass the new event to the roomserver and receive the correct event ID
	// event ID in case of duplicate transaction is discarded
	eventID, err := producer.SendEvents(
		req.Context(), []gomatrixserverlib.Event{*e}, cfg.Matrix.ServerName, txnAndDeviceID,
	)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	res := util.JSONResponse{
		Code: http.StatusOK,
		JSON: sendEventResponse{eventID},
	}
	// Add response to transactionsCache
	if txnID != nil {
		txnCache.AddTransaction(device.AccessToken, *txnID, &res)
	}

	return res
}

func generateSendEvent(
	req *http.Request,
	device *authtypes.Device,
	roomID, eventType string, stateKey *string,
	cfg config.Dendrite,
	queryAPI api.RoomserverQueryAPI,
) (*gomatrixserverlib.Event, *util.JSONResponse) {
	// parse the incoming http request
	userID := device.UserID
	var r map[string]interface{} // must be a JSON object
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return nil, resErr
	}

	evTime, err := httputil.ParseTSParam(req)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue(err.Error()),
		}
	}

	// create the new event and set all the fields we can
	builder := gomatrixserverlib.EventBuilder{
		Sender:   userID,
		RoomID:   roomID,
		Type:     eventType,
		StateKey: stateKey,
	}
	err = builder.SetContent(r)
	if err != nil {
		resErr := httputil.LogThenError(req, err)
		return nil, &resErr
	}

	var queryRes api.QueryLatestEventsAndStateResponse
	e, err := common.BuildEvent(req.Context(), &builder, cfg, evTime, queryAPI, &queryRes)
	if err == common.ErrRoomNoExists {
		return nil, &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Room does not exist"),
		}
	} else if err != nil {
		resErr := httputil.LogThenError(req, err)
		return nil, &resErr
	}

	// check to see if this user can perform this operation
	stateEvents := make([]*gomatrixserverlib.Event, len(queryRes.StateEvents))
	for i := range queryRes.StateEvents {
		stateEvents[i] = &queryRes.StateEvents[i]
	}
	provider := gomatrixserverlib.NewAuthEvents(stateEvents)
	if err = gomatrixserverlib.Allowed(*e, &provider); err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden(err.Error()), // TODO: Is this error string comprehensible to the client?
		}
	}
	return e, nil
}
