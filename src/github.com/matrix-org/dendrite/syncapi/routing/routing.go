// Copyright 2017 Vector Creations Ltd
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
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/syncapi/config"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
)

const pathPrefixR0 = "/_matrix/client/r0"

// SetupSyncServerListeners configures the given mux with sync-server listeners
func SetupSyncServerListeners(servMux *http.ServeMux, httpClient *http.Client, cfg config.Sync, srp *sync.RequestPool, deviceDB *devices.Database) {
	apiMux := mux.NewRouter()
	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()
	r0mux.Handle("/sync", common.MakeAuthAPI("sync", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
		return srp.OnIncomingSyncRequest(req, device)
	}))
	servMux.Handle("/metrics", prometheus.Handler())
	servMux.Handle("/api/", http.StripPrefix("/api", apiMux))
}
