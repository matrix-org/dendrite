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

	"github.com/matrix-org/dendrite/roomserver/api"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/publicroomsapi/directory"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/dendrite/publicroomsapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const pathPrefixR0 = "/_matrix/client/r0"

// Setup configures the given mux with publicroomsapi server listeners
//
// Due to Setup being used to call many other functions, a gocyclo nolint is
// applied:
// nolint: gocyclo
func Setup(
	apiMux *mux.Router, deviceDB devices.Database, publicRoomsDB storage.Database, rsAPI api.RoomserverInternalAPI,
	fedClient *gomatrixserverlib.FederationClient, extRoomsProvider types.ExternalPublicRoomsProvider,
) {
	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()

	authData := auth.Data{
		AccountDB:   nil,
		DeviceDB:    deviceDB,
		AppServices: nil,
	}

	r0mux.Handle("/directory/list/room/{roomID}",
		internal.MakeExternalAPI("directory_list", func(req *http.Request) util.JSONResponse {
			vars, err := internal.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return directory.GetVisibility(req, publicRoomsDB, vars["roomID"])
		}),
	).Methods(http.MethodGet, http.MethodOptions)
	// TODO: Add AS support
	r0mux.Handle("/directory/list/room/{roomID}",
		internal.MakeAuthAPI("directory_list", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars, err := internal.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return directory.SetVisibility(req, publicRoomsDB, rsAPI, device, vars["roomID"])
		}),
	).Methods(http.MethodPut, http.MethodOptions)
	r0mux.Handle("/publicRooms",
		internal.MakeExternalAPI("public_rooms", func(req *http.Request) util.JSONResponse {
			if extRoomsProvider != nil {
				return directory.GetPostPublicRoomsWithExternal(req, publicRoomsDB, fedClient, extRoomsProvider)
			}
			return directory.GetPostPublicRooms(req, publicRoomsDB)
		}),
	).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)

	// Federation - TODO: should this live here or in federation API? It's sure easier if it's here so here it is.
	apiMux.Handle("/_matrix/federation/v1/publicRooms",
		internal.MakeExternalAPI("federation_public_rooms", func(req *http.Request) util.JSONResponse {
			return directory.GetPostPublicRooms(req, publicRoomsDB)
		}),
	).Methods(http.MethodGet)
}
