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
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// GetTransactionFromRelay implements GET /_matrix/federation/v1/relay_txn/{userID}
// This endpoint can be extracted into a separate relay server service.
func GetTransactionFromRelay(
	httpReq *http.Request,
	fedReq *gomatrixserverlib.FederationRequest,
	relayAPI api.RelayInternalAPI,
	userID gomatrixserverlib.UserID,
) util.JSONResponse {
	logrus.Infof("Processing relay_txn for %s", userID.Raw())

	var previousEntry fclient.RelayEntry
	if err := json.Unmarshal(fedReq.Content(), &previousEntry); err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.BadJSON("invalid json provided"),
		}
	}
	if previousEntry.EntryID < 0 {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.BadJSON("Invalid entry id provided. Must be >= 0."),
		}
	}
	logrus.Infof("Previous entry provided: %v", previousEntry.EntryID)

	response, err := relayAPI.QueryTransactions(httpReq.Context(), userID, previousEntry)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: fclient.RespGetRelayTransaction{
			Transaction:   response.Transaction,
			EntryID:       response.EntryID,
			EntriesQueued: response.EntriesQueued,
		},
	}
}
