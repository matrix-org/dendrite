package routing

import (
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// ForwardAsync implements /_matrix/federation/v1/forward_async/{txnID}/{userID}
// This endpoint can be extracted into a separate relay server service.
func ForwardAsync(
	httpReq *http.Request,
	fedReq *gomatrixserverlib.FederationRequest,
	fedAPI api.FederationInternalAPI,
	txnID gomatrixserverlib.TransactionID,
	userID gomatrixserverlib.UserID,
) util.JSONResponse {
	var txnEvents struct {
		PDUs []json.RawMessage       `json:"pdus"`
		EDUs []gomatrixserverlib.EDU `json:"edus"`
	}

	if err := json.Unmarshal(fedReq.Content(), &txnEvents); err != nil {
		println("The request body could not be decoded into valid JSON. " + err.Error())
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotJSON("The request body could not be decoded into valid JSON. " + err.Error()),
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
	err := fedAPI.PerformStoreAsync(httpReq.Context(), &req, &res)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.BadJSON("could not store the transaction for forwarding"),
		}
	}

	return util.JSONResponse{Code: 200}
}
