package routing

import (
	"net/http"

	"github.com/matrix-org/dendrite/relayapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type AsyncEventsResponse struct {
	Transaction gomatrixserverlib.Transaction `json:"transaction"`
	Remaining   uint32                        `json:"remaining"`
}

// GetAsyncEvents implements /_matrix/federation/v1/async_events/{userID}
// This endpoint can be extracted into a separate relay server service.
func GetAsyncEvents(
	httpReq *http.Request,
	fedReq *gomatrixserverlib.FederationRequest,
	relayAPI api.RelayInternalAPI,
	userID gomatrixserverlib.UserID,
) util.JSONResponse {
	logrus.Infof("Handling async_events for %v", userID)
	var response api.QueryAsyncTransactionsResponse
	err := relayAPI.QueryAsyncTransactions(httpReq.Context(), &api.QueryAsyncTransactionsRequest{UserID: userID}, &response)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: AsyncEventsResponse{
			Transaction: response.Txn,
			Remaining:   response.RemainingCount,
		},
	}
}
