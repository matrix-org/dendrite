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

	userapi "github.com/matrix-org/dendrite/userapi/api"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const pathPrefixR0 = "/media/r0"

// Setup registers the media API HTTP handlers
//
// Due to Setup being used to call many other functions, a gocyclo nolint is
// applied:
// nolint: gocyclo
func Setup(
	publicAPIMux *mux.Router,
	cfg *config.Dendrite,
	db storage.Database,
	userAPI userapi.UserInternalAPI,
	client *gomatrixserverlib.Client,
) {
	r0mux := publicAPIMux.PathPrefix(pathPrefixR0).Subrouter()

	activeThumbnailGeneration := &types.ActiveThumbnailGeneration{
		PathToResult: map[string]*types.ThumbnailGenerationResult{},
	}
	r0mux.Handle("/upload", httputil.MakeAuthAPI(
		"upload", userAPI,
		func(req *http.Request, _ *userapi.Device) util.JSONResponse {
			return Upload(req, cfg, db, activeThumbnailGeneration)
		},
	)).Methods(http.MethodPost, http.MethodOptions)

	activeRemoteRequests := &types.ActiveRemoteRequests{
		MXCToResult: map[string]*types.RemoteRequestResult{},
	}
	r0mux.Handle("/download/{serverName}/{mediaId}",
		makeDownloadAPI("download", cfg, db, client, activeRemoteRequests, activeThumbnailGeneration),
	).Methods(http.MethodGet, http.MethodOptions)
	r0mux.Handle("/thumbnail/{serverName}/{mediaId}",
		makeDownloadAPI("thumbnail", cfg, db, client, activeRemoteRequests, activeThumbnailGeneration),
	).Methods(http.MethodGet, http.MethodOptions)
}

func makeDownloadAPI(
	name string,
	cfg *config.Dendrite,
	db storage.Database,
	client *gomatrixserverlib.Client,
	activeRemoteRequests *types.ActiveRemoteRequests,
	activeThumbnailGeneration *types.ActiveThumbnailGeneration,
) http.HandlerFunc {
	counterVec := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: name,
			Help: "Total number of media_api requests for either thumbnails or full downloads",
		},
		[]string{"code"},
	)
	httpHandler := func(w http.ResponseWriter, req *http.Request) {
		req = util.RequestWithLogging(req)

		// Set internal headers returned regardless of the outcome of the request
		util.SetCORSHeaders(w)
		// Content-Type will be overridden in case of returning file data, else we respond with JSON-formatted errors
		w.Header().Set("Content-Type", "application/json")
		vars, _ := httputil.URLDecodeMapValues(mux.Vars(req))
		Download(
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
	}
	return promhttp.InstrumentHandlerCounter(counterVec, http.HandlerFunc(httpHandler))
}
