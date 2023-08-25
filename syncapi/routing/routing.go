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
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/fulltext"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

// Setup configures the given mux with sync-server listeners
//
// Due to Setup being used to call many other functions, a gocyclo nolint is
// applied:
// nolint: gocyclo
func Setup(
	csMux *mux.Router, srp *sync.RequestPool, syncDB storage.Database,
	userAPI userapi.SyncUserAPI,
	rsAPI api.SyncRoomserverAPI,
	cfg *config.SyncAPI,
	lazyLoadCache caching.LazyLoadCache,
	fts fulltext.Indexer,
	rateLimits *httputil.RateLimits,
) {
	v1unstablemux := csMux.PathPrefix("/{apiversion:(?:v1|unstable)}/").Subrouter()
	v3mux := csMux.PathPrefix("/{apiversion:(?:r0|v3)}/").Subrouter()

	// TODO: Add AS support for all handlers below.
	v3mux.Handle("/sync", httputil.MakeAuthAPI("sync", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		return srp.OnIncomingSyncRequest(req, device)
	}, httputil.WithAllowGuests())).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/rooms/{roomID}/messages", httputil.MakeAuthAPI("room_messages", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		// not specced, but ensure we're rate limiting requests to this endpoint
		if r := rateLimits.Limit(req, device); r != nil {
			return *r
		}
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		return OnIncomingMessagesRequest(req, syncDB, vars["roomID"], device, rsAPI, cfg, srp, lazyLoadCache)
	}, httputil.WithAllowGuests())).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/rooms/{roomID}/event/{eventID}",
		httputil.MakeAuthAPI("rooms_get_event", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetEvent(req, device, vars["roomID"], vars["eventID"], cfg, syncDB, rsAPI)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/user/{userId}/filter",
		httputil.MakeAuthAPI("put_filter", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return PutFilter(req, device, syncDB, vars["userId"])
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	v3mux.Handle("/user/{userId}/filter/{filterId}",
		httputil.MakeAuthAPI("get_filter", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetFilter(req, device, syncDB, vars["userId"], vars["filterId"])
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/keys/changes", httputil.MakeAuthAPI("keys_changes", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		return srp.OnIncomingKeyChangeRequest(req, device)
	}, httputil.WithAllowGuests())).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/rooms/{roomId}/context/{eventId}",
		httputil.MakeAuthAPI("context", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}

			return Context(
				req, device,
				rsAPI, syncDB,
				vars["roomId"], vars["eventId"],
				lazyLoadCache,
			)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v1unstablemux.Handle("/rooms/{roomId}/relations/{eventId}",
		httputil.MakeAuthAPI("relations", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}

			return Relations(
				req, device, syncDB, rsAPI,
				vars["roomId"], vars["eventId"], "", "",
			)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v1unstablemux.Handle("/rooms/{roomId}/relations/{eventId}/{relType}",
		httputil.MakeAuthAPI("relation_type", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}

			return Relations(
				req, device, syncDB, rsAPI,
				vars["roomId"], vars["eventId"], vars["relType"], "",
			)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v1unstablemux.Handle("/rooms/{roomId}/relations/{eventId}/{relType}/{eventType}",
		httputil.MakeAuthAPI("relation_type_event", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}

			return Relations(
				req, device, syncDB, rsAPI,
				vars["roomId"], vars["eventId"], vars["relType"], vars["eventType"],
			)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/search",
		httputil.MakeAuthAPI("search", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if !cfg.Fulltext.Enabled {
				return util.JSONResponse{
					Code: http.StatusNotImplemented,
					JSON: spec.Unknown("Search has been disabled by the server administrator."),
				}
			}
			var nextBatch *string
			if err := req.ParseForm(); err != nil {
				return util.JSONResponse{
					Code: http.StatusInternalServerError,
					JSON: spec.InternalServerError{},
				}
			}
			if req.Form.Has("next_batch") {
				nb := req.FormValue("next_batch")
				nextBatch = &nb
			}
			return Search(req, device, syncDB, fts, nextBatch, rsAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	v3mux.Handle("/rooms/{roomID}/members",
		httputil.MakeAuthAPI("rooms_members", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			var membership, notMembership *string
			if req.URL.Query().Has("membership") {
				m := req.URL.Query().Get("membership")
				membership = &m
			}
			if req.URL.Query().Has("not_membership") {
				m := req.URL.Query().Get("not_membership")
				notMembership = &m
			}

			at := req.URL.Query().Get("at")
			return GetMemberships(req, device, vars["roomID"], syncDB, rsAPI, false, membership, notMembership, at)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/rooms/{roomID}/joined_members",
		httputil.MakeAuthAPI("rooms_members", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			at := req.URL.Query().Get("at")
			membership := spec.Join
			return GetMemberships(req, device, vars["roomID"], syncDB, rsAPI, true, &membership, nil, at)
		}),
	).Methods(http.MethodGet, http.MethodOptions)
}
