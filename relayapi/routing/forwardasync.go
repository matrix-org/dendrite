// Copyright 2022 The Matrix.org Foundation C.I.C.
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
	"github.com/matrix-org/dendrite/relayapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// ForwardAsync implements /_matrix/federation/v1/forward_async/{txnID}/{userID}
// This endpoint can be extracted into a separate relay server service.
func ForwardAsync(
	httpReq *http.Request,
	fedReq *gomatrixserverlib.FederationRequest,
	relayAPI api.RelayInternalAPI,
	txnID gomatrixserverlib.TransactionID,
	userID gomatrixserverlib.UserID,
) util.JSONResponse {
	var txnEvents struct {
		PDUs []json.RawMessage       `json:"pdus"`
		EDUs []gomatrixserverlib.EDU `json:"edus"`
	}

	if err := json.Unmarshal(fedReq.Content(), &txnEvents); err != nil {
		logrus.Info("The request body could not be decoded into valid JSON." + err.Error())
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotJSON("The request body could not be decoded into valid JSON." + err.Error()),
		}
	}

	// Transactions are limited in size; they can have at most 50 PDUs and 100 EDUs.
	// https://matrix.org/docs/spec/server_server/latest#transactions
	if len(txnEvents.PDUs) > 50 || len(txnEvents.EDUs) > 100 {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("max 50 pdus / 100 edus"),
		}
	}

	t := gomatrixserverlib.Transaction{}
	t.PDUs = txnEvents.PDUs
	t.EDUs = txnEvents.EDUs
	t.Origin = fedReq.Origin()
	t.TransactionID = txnID
	t.Destination = userID.Domain()

	util.GetLogger(httpReq.Context()).Warnf("Received transaction %q from %q containing %d PDUs, %d EDUs", txnID, fedReq.Origin(), len(t.PDUs), len(t.EDUs))

	req := api.PerformStoreAsyncRequest{
		Txn:    t,
		UserID: userID,
	}
	res := api.PerformStoreAsyncResponse{}
	err := relayAPI.PerformStoreAsync(httpReq.Context(), &req, &res)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.BadJSON("could not store the transaction for forwarding"),
		}
	}

	return util.JSONResponse{Code: 200}
}
