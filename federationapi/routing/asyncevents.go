package routing

import (
	"net/http"

	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type AsyncEventsResponse struct {
	Transaction gomatrixserverlib.Transaction `json:"transaction"`
	Remaining   uint32                        `json:"remaining"`
}

// GetAsyncEvents implements /_matrix/federation/v1/async_events/{userID}
func GetAsyncEvents(
	httpReq *http.Request,
	fedAPI api.FederationInternalAPI,
	userID gomatrixserverlib.UserID,
) util.JSONResponse {
	var response api.QueryAsyncTransactionsResponse
	err := fedAPI.QueryAsyncTransactions(httpReq.Context(), &api.QueryAsyncTransactionsRequest{UserID: userID}, &response)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: AsyncEventsResponse{
			Transaction: gomatrixserverlib.Transaction{},
			Remaining:   response.RemainingCount,
		},
	}
}
