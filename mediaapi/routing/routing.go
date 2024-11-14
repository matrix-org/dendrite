// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/element-hq/dendrite/federationapi/routing"
	"github.com/element-hq/dendrite/internal/httputil"
	"github.com/element-hq/dendrite/mediaapi/storage"
	"github.com/element-hq/dendrite/mediaapi/types"
	"github.com/element-hq/dendrite/setup/config"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/gorilla/mux"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// configResponse is the response to GET /_matrix/media/r0/config
// https://matrix.org/docs/spec/client_server/latest#get-matrix-media-r0-config
type configResponse struct {
	UploadSize *config.FileSizeBytes `json:"m.upload.size,omitempty"`
}

// Setup registers the media API HTTP handlers
//
// Due to Setup being used to call many other functions, a gocyclo nolint is
// applied:
// nolint: gocyclo
func Setup(
	routers httputil.Routers,
	cfg *config.Dendrite,
	db storage.Database,
	userAPI userapi.MediaUserAPI,
	client *fclient.Client,
	federationClient fclient.FederationClient,
	keyRing gomatrixserverlib.JSONVerifier,
) {
	rateLimits := httputil.NewRateLimits(&cfg.ClientAPI.RateLimiting)

	v3mux := routers.Media.PathPrefix("/{apiversion:(?:r0|v1|v3)}/").Subrouter()
	v1mux := routers.Client.PathPrefix("/v1/media/").Subrouter()
	v1fedMux := routers.Federation.PathPrefix("/v1/media/").Subrouter()

	activeThumbnailGeneration := &types.ActiveThumbnailGeneration{
		PathToResult: map[string]*types.ThumbnailGenerationResult{},
	}

	uploadHandler := httputil.MakeAuthAPI(
		"upload", userAPI,
		func(req *http.Request, dev *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, dev); r != nil {
				return *r
			}
			return Upload(req, &cfg.MediaAPI, dev, db, activeThumbnailGeneration)
		},
	)

	configHandler := httputil.MakeAuthAPI("config", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		if r := rateLimits.Limit(req, device); r != nil {
			return *r
		}
		respondSize := &cfg.MediaAPI.MaxFileSizeBytes
		if cfg.MediaAPI.MaxFileSizeBytes == 0 {
			respondSize = nil
		}
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: configResponse{UploadSize: respondSize},
		}
	})

	v3mux.Handle("/upload", uploadHandler).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/config", configHandler).Methods(http.MethodGet, http.MethodOptions)

	activeRemoteRequests := &types.ActiveRemoteRequests{
		MXCToResult: map[string]*types.RemoteRequestResult{},
	}

	downloadHandler := makeDownloadAPI("download_unauthed", &cfg.MediaAPI, rateLimits, db, client, federationClient, activeRemoteRequests, activeThumbnailGeneration, false)
	v3mux.Handle("/download/{serverName}/{mediaId}", downloadHandler).Methods(http.MethodGet, http.MethodOptions)
	v3mux.Handle("/download/{serverName}/{mediaId}/{downloadName}", downloadHandler).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/thumbnail/{serverName}/{mediaId}",
		makeDownloadAPI("thumbnail_unauthed", &cfg.MediaAPI, rateLimits, db, client, federationClient, activeRemoteRequests, activeThumbnailGeneration, false),
	).Methods(http.MethodGet, http.MethodOptions)

	// v1 client endpoints requiring auth
	downloadHandlerAuthed := httputil.MakeHTTPAPI("download", userAPI, cfg.Global.Metrics.Enabled, makeDownloadAPI("download_authed_client", &cfg.MediaAPI, rateLimits, db, client, federationClient, activeRemoteRequests, activeThumbnailGeneration, false), httputil.WithAuth())
	v1mux.Handle("/config", configHandler).Methods(http.MethodGet, http.MethodOptions)
	v1mux.Handle("/download/{serverName}/{mediaId}", downloadHandlerAuthed).Methods(http.MethodGet, http.MethodOptions)
	v1mux.Handle("/download/{serverName}/{mediaId}/{downloadName}", downloadHandlerAuthed).Methods(http.MethodGet, http.MethodOptions)

	v1mux.Handle("/thumbnail/{serverName}/{mediaId}",
		httputil.MakeHTTPAPI("thumbnail", userAPI, cfg.Global.Metrics.Enabled, makeDownloadAPI("thumbnail_authed_client", &cfg.MediaAPI, rateLimits, db, client, federationClient, activeRemoteRequests, activeThumbnailGeneration, false), httputil.WithAuth()),
	).Methods(http.MethodGet, http.MethodOptions)

	// same, but for federation
	v1fedMux.Handle("/download/{mediaId}", routing.MakeFedHTTPAPI(cfg.Global.ServerName, cfg.Global.IsLocalServerName, keyRing,
		makeDownloadAPI("download_authed_federation", &cfg.MediaAPI, rateLimits, db, client, federationClient, activeRemoteRequests, activeThumbnailGeneration, true),
	)).Methods(http.MethodGet, http.MethodOptions)
	v1fedMux.Handle("/thumbnail/{mediaId}", routing.MakeFedHTTPAPI(cfg.Global.ServerName, cfg.Global.IsLocalServerName, keyRing,
		makeDownloadAPI("thumbnail_authed_federation", &cfg.MediaAPI, rateLimits, db, client, federationClient, activeRemoteRequests, activeThumbnailGeneration, true),
	)).Methods(http.MethodGet, http.MethodOptions)
}

var thumbnailCounter = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "dendrite",
		Subsystem: "mediaapi",
		Name:      "thumbnail",
		Help:      "Total number of media_api requests for thumbnails",
	},
	[]string{"code", "type"},
)

var thumbnailSize = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: "dendrite",
		Subsystem: "mediaapi",
		Name:      "thumbnail_size_bytes",
		Help:      "Total size of media_api requests for thumbnails",
		Buckets:   []float64{50, 100, 200, 500, 900, 1500, 3000, 6000},
	},
	[]string{"code", "type"},
)

var downloadCounter = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "dendrite",
		Subsystem: "mediaapi",
		Name:      "download",
		Help:      "Total size of media_api requests for full downloads",
	},
	[]string{"code", "type"},
)

var downloadSize = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: "dendrite",
		Subsystem: "mediaapi",
		Name:      "download_size_bytes",
		Help:      "Total size of media_api requests for full downloads",
		Buckets:   []float64{1500, 3000, 6000, 10_000, 50_000, 100_000},
	},
	[]string{"code", "type"},
)

func makeDownloadAPI(
	name string,
	cfg *config.MediaAPI,
	rateLimits *httputil.RateLimits,
	db storage.Database,
	client *fclient.Client,
	fedClient fclient.FederationClient,
	activeRemoteRequests *types.ActiveRemoteRequests,
	activeThumbnailGeneration *types.ActiveThumbnailGeneration,
	forFederation bool,
) http.HandlerFunc {
	var counterVec *prometheus.CounterVec
	var sizeVec *prometheus.HistogramVec
	var requestType string
	if cfg.Matrix.Metrics.Enabled {
		split := strings.Split(name, "_")
		// The first part of the split is either "download" or "thumbnail"
		name = split[0]
		// The remainder of the split is something like "authed_download" or "unauthed_thumbnail", etc.
		// This is used to curry the metrics with the given types.
		requestType = strings.Join(split[1:], "_")

		counterVec = thumbnailCounter
		sizeVec = thumbnailSize
		if name != "thumbnail" {
			counterVec = downloadCounter
			sizeVec = downloadSize
		}
	}
	httpHandler := func(w http.ResponseWriter, req *http.Request) {
		req = util.RequestWithLogging(req)

		// Set internal headers returned regardless of the outcome of the request
		util.SetCORSHeaders(w)
		w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
		// Content-Type will be overridden in case of returning file data, else we respond with JSON-formatted errors
		w.Header().Set("Content-Type", "application/json")

		// Ratelimit requests
		// NOTSPEC: The spec says everything at /media/ should be rate limited, but this causes issues with thumbnails (#2243)
		if name != "thumbnail" {
			if r := rateLimits.Limit(req, nil); r != nil {
				if err := json.NewEncoder(w).Encode(r); err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
		}

		vars, _ := httputil.URLDecodeMapValues(mux.Vars(req))
		serverName := spec.ServerName(vars["serverName"])

		// For the purposes of loop avoidance, we will return a 404 if allow_remote is set to
		// false in the query string and the target server name isn't our own.
		// https://github.com/matrix-org/matrix-doc/pull/1265
		if allowRemote := req.URL.Query().Get("allow_remote"); strings.ToLower(allowRemote) == "false" {
			if serverName != cfg.Matrix.ServerName {
				w.WriteHeader(http.StatusNotFound)
				return
			}
		}

		// Cache media for at least one day.
		w.Header().Set("Cache-Control", "public,max-age=86400,s-maxage=86400")

		Download(
			w,
			req,
			serverName,
			types.MediaID(vars["mediaId"]),
			cfg,
			db,
			client,
			fedClient,
			activeRemoteRequests,
			activeThumbnailGeneration,
			strings.HasPrefix(name, "thumbnail"),
			vars["downloadName"],
			forFederation,
		)
	}

	var handlerFunc http.HandlerFunc
	if counterVec != nil {
		counterVec = counterVec.MustCurryWith(prometheus.Labels{"type": requestType})
		sizeVec2 := sizeVec.MustCurryWith(prometheus.Labels{"type": requestType})
		handlerFunc = promhttp.InstrumentHandlerCounter(counterVec, http.HandlerFunc(httpHandler))
		handlerFunc = promhttp.InstrumentHandlerResponseSize(sizeVec2, handlerFunc).ServeHTTP
	} else {
		handlerFunc = http.HandlerFunc(httpHandler)
	}
	return handlerFunc
}
