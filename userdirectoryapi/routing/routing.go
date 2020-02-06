// Copyright 2019 Anton Stuetz
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

	"github.com/matrix-org/dendrite/userdirectoryapi/search"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/util"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
)

const pathPrefixR0 = "/_matrix/client/r0"

func Setup(apiMux *mux.Router, accountDB *accounts.Database, deviceDB *devices.Database) {
	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()

	authData := auth.Data{
		AccountDB:   accountDB,
		DeviceDB:    deviceDB,
		AppServices: nil,
	}

	r0mux.Handle("/user_directory/search",
		common.MakeAuthAPI("user_directory_search", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return search.Search(req, accountDB)
		}),
	).Methods(http.MethodPost)
}
