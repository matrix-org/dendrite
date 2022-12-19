package inthttp

import (
	"github.com/gorilla/mux"

	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/relayapi/api"
)

// AddRoutes adds the RelayInternalAPI handlers to the http.ServeMux.
// nolint:gocyclo
func AddRoutes(intAPI api.RelayInternalAPI, internalAPIMux *mux.Router, enableMetrics bool) {
	internalAPIMux.Handle(
		RelayAPIPerformRelayServerSyncPath,
		httputil.MakeInternalRPCAPI("RelayAPIPerformRelayServerSync", enableMetrics, intAPI.PerformRelayServerSync),
	)

	internalAPIMux.Handle(
		RelayAPIPerformStoreAsyncPath,
		httputil.MakeInternalRPCAPI("RelayAPIPerformStoreAsync", enableMetrics, intAPI.PerformStoreAsync),
	)

	internalAPIMux.Handle(
		RelayAPIQueryAsyncTransactionsPath,
		httputil.MakeInternalRPCAPI("RelayAPIQueryAsyncTransactions", enableMetrics, intAPI.QueryAsyncTransactions),
	)
}
