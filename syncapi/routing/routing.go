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
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/common"
	encryptoapi "github.com/matrix-org/dendrite/encryptoapi/storage"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/util"
)

const pathPrefixR0 = "/_matrix/client/r0"
const pathPrefixUnstable = "/_matrix/client/unstable"

// Setup configures the given mux with sync-server listeners
<<<<<<< HEAD:syncapi/routing/routing.go
//
// Due to Setup being used to call many other functions, a gocyclo nolint is
// applied:
// nolint: gocyclo
func Setup(apiMux *mux.Router, srp *sync.RequestPool, syncDB *storage.SyncServerDatasource, deviceDB *devices.Database) {
=======
func Setup(apiMux *mux.Router, srp *sync.RequestPool, syncDB *storage.SyncServerDatabase, deviceDB *devices.Database, notifier *sync.Notifier, encryptDB *encryptoapi.Database) {
>>>>>>> 8b4b3c6fc46900e9bfe5e234eda309200662b34a:src/github.com/matrix-org/dendrite/syncapi/routing/routing.go
	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()
	unstablemux := apiMux.PathPrefix(pathPrefixUnstable).Subrouter()

	authData := auth.Data{
		AccountDB:   nil,
		DeviceDB:    deviceDB,
		AppServices: nil,
	}

	// TODO: Add AS support for all handlers below.
	r0mux.Handle("/sync", common.MakeAuthAPI("sync", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
		return srp.OnIncomingSyncRequest(req, device, encryptDB)
	})).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/state", common.MakeAuthAPI("room_state", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
		vars, err := common.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		return OnIncomingStateRequest(req, syncDB, vars["roomID"])
	})).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/state/{type}", common.MakeAuthAPI("room_state", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
		vars, err := common.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		return OnIncomingStateTypeRequest(req, syncDB, vars["roomID"], vars["type"], "")
	})).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/state/{type}/{stateKey}", common.MakeAuthAPI("room_state", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
		vars, err := common.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		return OnIncomingStateTypeRequest(req, syncDB, vars["roomID"], vars["type"], vars["stateKey"])
	})).Methods(http.MethodGet, http.MethodOptions)

	unstablemux.Handle("/sendToDevice/{eventType}/{txnId}",
		common.MakeAuthAPI("look up changes", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			eventType := vars["eventType"]
			txnID := vars["txnId"]
			return SendToDevice(req, device.UserID, syncDB, deviceDB, eventType, txnID, notifier)
		}),
	).Methods(http.MethodPut, http.MethodOptions)
}
