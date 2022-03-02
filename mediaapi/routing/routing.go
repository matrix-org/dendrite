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
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// configResponse is the response to GET /_matrix/media/r0/config
// https://matrix.org/docs/spec/client_server/latest#get-matrix-media-r0-config
type configResponse struct {
	UploadSize config.FileSizeBytes `json:"m.upload.size"`
}

// Setup registers the media API HTTP handlers
//
// Due to Setup being used to call many other functions, a gocyclo nolint is
// applied:
// nolint: gocyclo
func Setup(
	publicAPIMux *mux.Router,
	cfg *config.MediaAPI,
	rateLimit *config.RateLimiting,
	db storage.Database,
	userAPI userapi.UserInternalAPI,
	client *gomatrixserverlib.Client,
) {
	rateLimits := httputil.NewRateLimits(rateLimit)

	v3mux := publicAPIMux.PathPrefix("/{apiversion:(?:r0|v1|v3)}/").Subrouter()

	activeThumbnailGeneration := &types.ActiveThumbnailGeneration{
		PathToResult: map[string]*types.ThumbnailGenerationResult{},
	}

	uploadHandler := httputil.MakeAuthAPI(
		"upload", userAPI,
		func(req *http.Request, dev *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req); r != nil {
				return *r
			}
			return Upload(req, cfg, dev, db, activeThumbnailGeneration)
		},
	)

	configHandler := httputil.MakeAuthAPI("config", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		if r := rateLimits.Limit(req); r != nil {
			return *r
		}
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: configResponse{UploadSize: *cfg.MaxFileSizeBytes},
		}
	})

	v3mux.Handle("/upload", uploadHandler).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/config", configHandler).Methods(http.MethodGet, http.MethodOptions)

	activeRemoteRequests := &types.ActiveRemoteRequests{
		MXCToResult: map[string]*types.RemoteRequestResult{},
	}

	downloadHandler := makeDownloadAPI("download", cfg, rateLimits, db, client, activeRemoteRequests, activeThumbnailGeneration)
	v3mux.Handle("/download/{serverName}/{mediaId}", downloadHandler).Methods(http.MethodGet, http.MethodOptions)
	v3mux.Handle("/download/{serverName}/{mediaId}/{downloadName}", downloadHandler).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/thumbnail/{serverName}/{mediaId}",
		makeDownloadAPI("thumbnail", cfg, rateLimits, db, client, activeRemoteRequests, activeThumbnailGeneration),
	).Methods(http.MethodGet, http.MethodOptions)
}

func makeDownloadAPI(
	name string,
	cfg *config.MediaAPI,
	rateLimits *httputil.RateLimits,
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

		// Ratelimit requests
		if r := rateLimits.Limit(req); r != nil {
			if err := json.NewEncoder(w).Encode(r); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		vars, _ := httputil.URLDecodeMapValues(mux.Vars(req))
		serverName := gomatrixserverlib.ServerName(vars["serverName"])

		// For the purposes of loop avoidance, we will return a 404 if allow_remote is set to
		// false in the query string and the target server name isn't our own.
		// https://github.com/matrix-org/matrix-doc/pull/1265
		if allowRemote := req.URL.Query().Get("allow_remote"); strings.ToLower(allowRemote) == "false" {
			if serverName != cfg.Matrix.ServerName {
				w.WriteHeader(http.StatusNotFound)
				return
			}
		}

		Download(
			w,
			req,
			serverName,
			types.MediaID(vars["mediaId"]),
			cfg,
			db,
			client,
			activeRemoteRequests,
			activeThumbnailGeneration,
			name == "thumbnail",
			vars["downloadName"],
		)
	}
	return promhttp.InstrumentHandlerCounter(counterVec, http.HandlerFunc(httpHandler))
}
