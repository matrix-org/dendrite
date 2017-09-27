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

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/dendrite/mediaapi/writers"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
)

const pathPrefixR0 = "/_matrix/media/v1"

// Setup registers the media API HTTP handlers
func Setup(
	apiMux *mux.Router,
	cfg *config.Dendrite,
	db *storage.Database,
	deviceDB *devices.Database,
	client *gomatrixserverlib.Client,
) {
	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()

	activeThumbnailGeneration := &types.ActiveThumbnailGeneration{
		PathToResult: map[string]*types.ThumbnailGenerationResult{},
	}

	r0mux.Handle("/upload", common.MakeAuthAPI(
		"upload",
		deviceDB,
		func(req *http.Request, _ *authtypes.Device) util.JSONResponse {
			return writers.Upload(req, cfg, db, activeThumbnailGeneration)
		},
	)).Methods("POST", "OPTIONS")

	activeRemoteRequests := &types.ActiveRemoteRequests{
		MXCToResult: map[string]*types.RemoteRequestResult{},
	}
	r0mux.Handle("/download/{serverName}/{mediaId}",
		makeDownloadAPI("download", cfg, db, client, activeRemoteRequests, activeThumbnailGeneration),
	).Methods("GET")
	r0mux.Handle("/thumbnail/{serverName}/{mediaId}",
		makeDownloadAPI("thumbnail", cfg, db, client, activeRemoteRequests, activeThumbnailGeneration),
	).Methods("GET")
}

func makeDownloadAPI(
	name string,
	cfg *config.Dendrite,
	db *storage.Database,
	client *gomatrixserverlib.Client,
	activeRemoteRequests *types.ActiveRemoteRequests,
	activeThumbnailGeneration *types.ActiveThumbnailGeneration,
) http.HandlerFunc {
	return prometheus.InstrumentHandler(name, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req = util.RequestWithLogging(req)

		// Set common headers returned regardless of the outcome of the request
		util.SetCORSHeaders(w)
		// Content-Type will be overridden in case of returning file data, else we respond with JSON-formatted errors
		w.Header().Set("Content-Type", "application/json")

		vars := mux.Vars(req)
		writers.Download(
			w,
			req,
			gomatrixserverlib.ServerName(vars["serverName"]),
			types.MediaID(vars["mediaId"]),
			cfg,
			db,
			client,
			activeRemoteRequests,
			activeThumbnailGeneration,
			name == "thumbnail",
		)
	}))
}
