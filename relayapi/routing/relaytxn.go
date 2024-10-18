// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"encoding/json"
	"net/http"

	"github.com/element-hq/dendrite/relayapi/api"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// GetTransactionFromRelay implements GET /_matrix/federation/v1/relay_txn/{userID}
// This endpoint can be extracted into a separate relay server service.
func GetTransactionFromRelay(
	httpReq *http.Request,
	fedReq *fclient.FederationRequest,
	relayAPI api.RelayInternalAPI,
	userID spec.UserID,
) util.JSONResponse {
	logrus.Infof("Processing relay_txn for %s", userID.String())

	var previousEntry fclient.RelayEntry
	if err := json.Unmarshal(fedReq.Content(), &previousEntry); err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.BadJSON("invalid json provided"),
		}
	}
	if previousEntry.EntryID < 0 {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.BadJSON("Invalid entry id provided. Must be >= 0."),
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
