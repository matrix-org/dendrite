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
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/encryptoapi/storage"
	"github.com/matrix-org/util"
)

const pathPrefixUnstable = "/_matrix/client/unstable"

// Setup works for setting up encryption api server
func Setup(
	apiMux *mux.Router,
	encryptionDB *storage.Database,
	deviceDB *devices.Database,
) {
	authData := auth.Data{nil, deviceDB, nil}
	unstablemux := apiMux.PathPrefix(pathPrefixUnstable).Subrouter()

	unstablemux.Handle("/keys/upload/{deviceID}",
		common.MakeAuthAPI("upload keys", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return UploadPKeys(req, encryptionDB, device.UserID, device.ID)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	unstablemux.Handle("/keys/upload",
		common.MakeAuthAPI("upload keys", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return UploadPKeys(req, encryptionDB, device.UserID, device.ID)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	unstablemux.Handle("/keys/query",
		common.MakeAuthAPI("query keys", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			//vars := mux.Vars(req)
			return QueryPKeys(req, encryptionDB, device.ID, deviceDB)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	unstablemux.Handle("/keys/claim",
		common.MakeAuthAPI("claim keys", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return ClaimOneTimeKeys(req, encryptionDB)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

}
