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
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// Setup configures the given mux with sync-server listeners
//
// Due to Setup being used to call many other functions, a gocyclo nolint is
// applied:
// nolint: gocyclo
func Setup(
	csMux *mux.Router, srp *sync.RequestPool, syncDB storage.Database,
	userAPI userapi.UserInternalAPI, federation *gomatrixserverlib.FederationClient,
	rsAPI api.RoomserverInternalAPI,
	cfg *config.SyncAPI,
) {
	r0mux := csMux.PathPrefix("/r0").Subrouter()

	// TODO: Add AS support for all handlers below.
	// asdf
	r0mux.Handle("/sync", httputil.MakeAuthAPI("sync", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		return srp.OnIncomingSyncRequest(req, device)
	})).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/messages", httputil.MakeAuthAPI("room_messages", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		return OnIncomingMessagesRequest(req, syncDB, vars["roomID"], device, federation, rsAPI, cfg, srp)
	})).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/user/{userId}/filter",
		httputil.MakeAuthAPI("put_filter", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return PutFilter(req, device, syncDB, vars["userId"])
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	r0mux.Handle("/user/{userId}/filter/{filterId}",
		httputil.MakeAuthAPI("get_filter", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetFilter(req, device, syncDB, vars["userId"], vars["filterId"])
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/keys/changes", httputil.MakeAuthAPI("keys_changes", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		return srp.OnIncomingKeyChangeRequest(req, device)
	})).Methods(http.MethodGet, http.MethodOptions)
}
