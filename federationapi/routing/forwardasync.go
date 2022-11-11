package routing

import (
	"net/http"

	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// ForwardAsync implements /_matrix/federation/v1/forward_async/{txnID}/{userID}
func ForwardAsync(
	httpReq *http.Request,
	fedReq *gomatrixserverlib.FederationRequest,
	fedAPI api.FederationInternalAPI,
	txnId gomatrixserverlib.TransactionID,
	userID gomatrixserverlib.UserID,
) util.JSONResponse {

	// TODO: wrap in fedAPI call
	// fedAPI.db.AssociateAsyncTransactionWithDestinations(context.TODO(), userID, nil)

	return util.JSONResponse{Code: 200}
}