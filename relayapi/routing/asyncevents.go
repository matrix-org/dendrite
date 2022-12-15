package routing

import (
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/relayapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type AsyncEventsResponse struct {
	Txn           gomatrixserverlib.Transaction `json:"transaction"`
	EntryID       int64                         `json:"entry_id,omitempty"`
	EntriesQueued bool                          `json:"entries_queued"`
}

// GetAsyncEvents implements /_matrix/federation/v1/async_events/{userID}
// This endpoint can be extracted into a separate relay server service.
func GetAsyncEvents(
	httpReq *http.Request,
	fedReq *gomatrixserverlib.FederationRequest,
	relayAPI api.RelayInternalAPI,
	userID gomatrixserverlib.UserID,
) util.JSONResponse {
	logrus.Infof("Handling async_events for %s", userID.Raw())

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
		JSON: AsyncEventsResponse{
			Txn:           response.Txn,
			EntryID:       response.EntryID,
			EntriesQueued: response.EntriesQueued,
		},
	}
}
