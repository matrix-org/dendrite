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

	"github.com/matrix-org/dendrite/relayapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type RelayTxnResponse struct {
	Txn           gomatrixserverlib.Transaction `json:"transaction"`
	EntryID       int64                         `json:"entry_id,omitempty"`
	EntriesQueued bool                          `json:"entries_queued"`
}

// GetTxnFromRelay implements /_matrix/federation/v1/relay_txn/{userID}
// This endpoint can be extracted into a separate relay server service.
func GetTxnFromRelay(
	httpReq *http.Request,
	fedReq *gomatrixserverlib.FederationRequest,
	relayAPI api.RelayInternalAPI,
	userID gomatrixserverlib.UserID,
) util.JSONResponse {
	logrus.Infof("Handling relay_txn for %s", userID.Raw())

	entryProvided := false
	var previousEntry gomatrixserverlib.RelayEntry
	if err := json.Unmarshal(fedReq.Content(), &previousEntry); err == nil {
		logrus.Infof("Previous entry provided: %v", previousEntry.EntryID)
		entryProvided = true
	}

	request := api.QueryAsyncTransactionsRequest{
		UserID:        userID,
		PreviousEntry: gomatrixserverlib.RelayEntry{EntryID: -1},
	}
	if entryProvided {
		request.PreviousEntry = previousEntry
	}
	var response api.QueryAsyncTransactionsResponse
	err := relayAPI.QueryAsyncTransactions(
		httpReq.Context(),
		&request,
		&response)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: RelayTxnResponse{
			Txn:           response.Txn,
			EntryID:       response.EntryID,
			EntriesQueued: response.EntriesQueued,
		},
	}
}
