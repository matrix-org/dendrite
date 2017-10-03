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
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/util"
)

const pathPrefixR0 = "/_matrix/client/r0"

// Setup configures the given mux with sync-server listeners
func Setup(
	apiMux *mux.Router,
	srp *sync.RequestPool,
	deviceDB *devices.Database,
	syncDB *storage.SyncServerDatabase,
) {
	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()

	r0mux.Handle("/sync", common.MakeAuthAPI("sync", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
		return srp.OnIncomingSyncRequest(req, device)
	})).Methods("GET")

	r0mux.Handle("/rooms/{roomID}/state", common.MakeAuthAPI("room_state", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
		vars := mux.Vars(req)
		return srp.OnIncomingStateRequest(req, vars["roomID"])
	})).Methods("GET")

	// Stub implementations for sytest
	r0mux.Handle("/events",
		common.MakeExternalAPI("events", func(req *http.Request) util.JSONResponse {
			return util.JSONResponse{Code: 200, JSON: map[string]interface{}{
				"chunk": []interface{}{},
				"start": "",
				"end":   "",
			}}
		}),
	).Methods("GET")

	r0mux.Handle("/initialSync",
		common.MakeAuthAPI("initial_sync", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return sync.GetInitialSync(
				req, device, syncDB,
			)
		}),
	).Methods("GET")
}
