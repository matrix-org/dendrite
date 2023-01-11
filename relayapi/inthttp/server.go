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
		RelayAPIPerformStoreTransactionPath,
		httputil.MakeInternalRPCAPI("RelayAPIPerformStoreTransaction", enableMetrics, intAPI.PerformStoreTransaction),
	)

	internalAPIMux.Handle(
		RelayAPIQueryTransactionsPath,
		httputil.MakeInternalRPCAPI("RelayAPIQueryTransactions", enableMetrics, intAPI.QueryTransactions),
	)
}
