// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/util"
)

const pathPrefixR0 = "/client/r0"

// Setup registers HTTP handlers with the given ServeMux. It also supplies the given http.Client
// to clients which need to make outbound HTTP requests.
//
// Due to Setup being used to call many other functions, a gocyclo nolint is
// applied:
// nolint: gocyclo
func Setup(
	publicAPIMux *mux.Router, cfg *config.Dendrite,
	accountDB accounts.Database,
	deviceDB devices.Database,
) {
	r0mux := publicAPIMux.PathPrefix(pathPrefixR0).Subrouter()

	authData := auth.Data{
		AccountDB:   accountDB,
		DeviceDB:    deviceDB,
		AppServices: cfg.Derived.ApplicationServices,
	}

	r0mux.Handle("/keys/query",
		httputil.MakeAuthAPI("queryKeys", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return QueryKeys(req)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
}
