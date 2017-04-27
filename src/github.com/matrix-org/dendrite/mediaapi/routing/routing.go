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
	"context"
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/mediaapi/config"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/writers"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
)

const pathPrefixR0 = "/_matrix/media/v1"

type contextKeys string

const ctxValueLogger = contextKeys("logger")
const ctxValueRequestID = contextKeys("requestid")

type Fudge struct {
	Config         config.MediaAPI
	Database       *storage.Database
	DownloadServer writers.DownloadServer
}

func (fudge Fudge) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// NOTE: The code below is from util.Protect and respond but this is the only
	// API that needs a different form of it to be able to pass the
	// http.ResponseWriter to the handler
	reqID := util.RandomString(12)
	// Set a Logger and request ID on the context
	ctx := context.WithValue(req.Context(), ctxValueLogger, log.WithFields(log.Fields{
		"req.method": req.Method,
		"req.path":   req.URL.Path,
		"req.id":     reqID,
	}))
	ctx = context.WithValue(ctx, ctxValueRequestID, reqID)
	req = req.WithContext(ctx)

	logger := util.GetLogger(req.Context())
	logger.Print("Incoming request")

	if req.Method == "OPTIONS" {
		util.SetCORSHeaders(w)
		w.WriteHeader(200)
		return
	}

	// Set common headers returned regardless of the outcome of the request
	util.SetCORSHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(req)
	writers.Download(w, req, vars["serverName"], vars["mediaId"], fudge.Config, fudge.Database, fudge.DownloadServer)
}

// Setup registers HTTP handlers with the given ServeMux. It also supplies the given http.Client
// to clients which need to make outbound HTTP requests.
func Setup(servMux *http.ServeMux, httpClient *http.Client, cfg config.MediaAPI, db *storage.Database, repo *storage.Repository) {
	apiMux := mux.NewRouter()
	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()
	r0mux.Handle("/upload", make("upload", util.NewJSONRequestHandler(func(req *http.Request) util.JSONResponse {
		return writers.Upload(req, cfg, db, repo)
	})))

	downloadServer := writers.DownloadServer{
		Repository:      *repo,
		LocalServerName: cfg.ServerName,
	}

	fudge := Fudge{
		Config:         cfg,
		Database:       db,
		DownloadServer: downloadServer,
	}

	r0mux.Handle("/download/{serverName}/{mediaId}",
		prometheus.InstrumentHandler("download", fudge),
	)

	servMux.Handle("/metrics", prometheus.Handler())
	servMux.Handle("/api/", http.StripPrefix("/api", apiMux))
}

// make a util.JSONRequestHandler into an http.Handler
func make(metricsName string, h util.JSONRequestHandler) http.Handler {
	return prometheus.InstrumentHandler(metricsName, util.MakeJSONAPI(h))
}
