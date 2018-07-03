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
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/encryptoapi/storage"
	"github.com/matrix-org/util"
)

const pathPrefixR0 = "/_matrix/client/r0"
const pathPrefixUnstable = "/_matrix/client/unstable"

func Setup(
	apiMux *mux.Router,
	cfg config.Dendrite,
	encryptionDB *storage.Database,
	accountDB *accounts.Database,
	deviceDB *devices.Database,
) {
	//r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()
	unstablemux := apiMux.PathPrefix(pathPrefixUnstable).Subrouter()

	unstablemux.Handle("/keys/upload/{deviceID}",
		common.MakeAuthAPI("upload keys", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return UploadPKeys(req, encryptionDB, device.UserID, device.ID)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	unstablemux.Handle("/keys/upload",
		common.MakeAuthAPI("upload keys", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return UploadPKeys(req, encryptionDB, device.UserID, device.ID)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	unstablemux.Handle("/keys/query",
		common.MakeAuthAPI("query keys", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			//vars := mux.Vars(req)
			return QueryPKeys(req, encryptionDB, device.UserID, device.ID, deviceDB)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	unstablemux.Handle("/keys/claim",
		common.MakeAuthAPI("claim keys", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			//vars := mux.Vars(req)
			return ClaimOneTimeKeys(req, encryptionDB, device.UserID, device.ID, deviceDB)
		}),
	).Methods(http.MethodPost, http.MethodOptions)


}
