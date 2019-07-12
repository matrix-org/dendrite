// Copyright 2018 Vector Creations Ltd
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
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/common/transactions"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const pathPrefixApp = "/_matrix/app/r0"

// Setup registers HTTP handlers with the given ServeMux. It also supplies the given http.Client
// to clients which need to make outbound HTTP requests.
//
// Due to Setup being used to call many other functions, a gocyclo nolint is
// applied:
// nolint: gocyclo
func Setup(
	apiMux *mux.Router, cfg config.Dendrite, // nolint: unparam
	queryAPI api.RoomserverQueryAPI, aliasAPI api.RoomserverAliasAPI, // nolint: unparam
	accountDB *accounts.Database, // nolint: unparam
	federation *gomatrixserverlib.FederationClient, // nolint: unparam
	transactionsCache *transactions.Cache, // nolint: unparam
) {
	appMux := apiMux.PathPrefix(pathPrefixApp).Subrouter()

	appMux.Handle("/alias",
		common.MakeExternalAPI("alias", func(req *http.Request) util.JSONResponse {
			// TODO: Implement
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: nil,
			}
		}),
	).Methods(http.MethodGet, http.MethodOptions)
	appMux.Handle("/user",
		common.MakeExternalAPI("user", func(req *http.Request) util.JSONResponse {
			// TODO: Implement
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: nil,
			}
		}),
	).Methods(http.MethodGet, http.MethodOptions)
}
