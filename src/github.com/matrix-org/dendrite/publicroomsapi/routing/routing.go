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
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/publicroomsapi/directory"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/util"
)

const pathPrefixR0 = "/_matrix/client/r0"

// Setup configures the given mux with publicroomsapi server listeners
func Setup(apiMux *mux.Router, publicRoomsDB *storage.PublicRoomsServerDatabase) {
	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()
	r0mux.Handle("/directory/list/room/{roomID}",
		common.MakeAPI("directory_list", func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			return directory.GetVisibility(req, publicRoomsDB, vars["roomID"])
		}),
	).Methods("GET")
	r0mux.Handle("/directory/list/room/{roomID}",
		common.MakeAPI("directory_list", func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			return directory.SetVisibility(req, publicRoomsDB, vars["roomID"])
		}),
	).Methods("PUT", "OPTIONS")
	r0mux.Handle("/publicRooms",
		common.MakeAPI("public_rooms", func(req *http.Request) util.JSONResponse {
			return directory.GetPublicRooms(req, publicRoomsDB)
		}),
	).Methods("GET", "POST", "OPTIONS")
}
