package inthttp

import (
	"github.com/gorilla/mux"

	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/relayapi/api"
)

// AddRoutes adds the RelayInternalAPI handlers to the http.ServeMux.
// nolint:gocyclo
func AddRoutes(intAPI api.RelayInternalAPI, internalAPIMux *mux.Router) {
	internalAPIMux.Handle(
		RelayAPIPerformRelayServerSyncPath,
		httputil.MakeInternalRPCAPI("RelayAPIPerformRelayServerSync", intAPI.PerformRelayServerSync),
	)

	internalAPIMux.Handle(
		RelayAPIPerformStoreAsyncPath,
		httputil.MakeInternalRPCAPI("RelayAPIPerformStoreAsync", intAPI.PerformStoreAsync),
	)

	internalAPIMux.Handle(
		RelayAPIQueryAsyncTransactionsPath,
		httputil.MakeInternalRPCAPI("RelayAPIQueryAsyncTransactions", intAPI.QueryAsyncTransactions),
	)
}
