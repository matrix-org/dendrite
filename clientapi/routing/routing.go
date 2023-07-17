// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"strings"

	"github.com/gorilla/mux"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"

	"github.com/matrix-org/dendrite/setup/base"
	userapi "github.com/matrix-org/dendrite/userapi/api"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/api"
	"github.com/matrix-org/dendrite/clientapi/auth"
	clientutil "github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/producers"
	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/transactions"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
)

// Setup registers HTTP handlers with the given ServeMux. It also supplies the given http.Client
// to clients which need to make outbound HTTP requests.
//
// Due to Setup being used to call many other functions, a gocyclo nolint is
// applied:
// nolint: gocyclo
func Setup(
	routers httputil.Routers,
	dendriteCfg *config.Dendrite,
	rsAPI roomserverAPI.ClientRoomserverAPI,
	asAPI appserviceAPI.AppServiceInternalAPI,
	userAPI userapi.ClientUserAPI,
	userDirectoryProvider userapi.QuerySearchProfilesAPI,
	federation fclient.FederationClient,
	syncProducer *producers.SyncAPIProducer,
	transactionsCache *transactions.Cache,
	federationSender federationAPI.ClientFederationAPI,
	extRoomsProvider api.ExtraPublicRoomsProvider,
	natsClient *nats.Conn, enableMetrics bool,
) {
	cfg := &dendriteCfg.ClientAPI
	mscCfg := &dendriteCfg.MSCs
	publicAPIMux := routers.Client
	wkMux := routers.WellKnown
	synapseAdminRouter := routers.SynapseAdmin
	dendriteAdminRouter := routers.DendriteAdmin

	if enableMetrics {
		prometheus.MustRegister(amtRegUsers, sendEventDuration)
	}

	rateLimits := httputil.NewRateLimits(&cfg.RateLimiting)
	userInteractiveAuth := auth.NewUserInteractive(userAPI, cfg)

	unstableFeatures := map[string]bool{
		"org.matrix.e2e_cross_signing": true,
		"org.matrix.msc2285.stable":    true,
	}
	for _, msc := range cfg.MSCs.MSCs {
		unstableFeatures["org.matrix."+msc] = true
	}

	// singleflight protects /join endpoints from being invoked
	// multiple times from the same user and room, otherwise
	// a state reset can occur. This also avoids unneeded
	// state calculations.
	// TODO: actually fix this in the roomserver, as there are
	// 		 possibly other ways that can result in a stat reset.
	sf := singleflight.Group{}

	if cfg.Matrix.WellKnownClientName != "" {
		logrus.Infof("Setting m.homeserver base_url as %s at /.well-known/matrix/client", cfg.Matrix.WellKnownClientName)
		wkMux.Handle("/client", httputil.MakeExternalAPI("wellknown", func(r *http.Request) util.JSONResponse {
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: struct {
					HomeserverName struct {
						BaseUrl string `json:"base_url"`
					} `json:"m.homeserver"`
				}{
					HomeserverName: struct {
						BaseUrl string `json:"base_url"`
					}{
						BaseUrl: cfg.Matrix.WellKnownClientName,
					},
				},
			}
		})).Methods(http.MethodGet, http.MethodOptions)
	}

	publicAPIMux.Handle("/versions",
		httputil.MakeExternalAPI("versions", func(req *http.Request) util.JSONResponse {
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: struct {
					Versions         []string        `json:"versions"`
					UnstableFeatures map[string]bool `json:"unstable_features"`
				}{Versions: []string{
					"r0.0.1",
					"r0.1.0",
					"r0.2.0",
					"r0.3.0",
					"r0.4.0",
					"r0.5.0",
					"r0.6.1",
					"v1.0",
					"v1.1",
					"v1.2",
				}, UnstableFeatures: unstableFeatures},
			}
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	if cfg.RegistrationSharedSecret != "" {
		logrus.Info("Enabling shared secret registration at /_synapse/admin/v1/register")
		sr := NewSharedSecretRegistration(cfg.RegistrationSharedSecret)
		synapseAdminRouter.Handle("/admin/v1/register",
			httputil.MakeExternalAPI("shared_secret_registration", func(req *http.Request) util.JSONResponse {
				if req.Method == http.MethodGet {
					return util.JSONResponse{
						Code: 200,
						JSON: struct {
							Nonce string `json:"nonce"`
						}{
							Nonce: sr.GenerateNonce(),
						},
					}
				}
				if req.Method == http.MethodPost {
					return handleSharedSecretRegistration(cfg, userAPI, sr, req)
				}
				return util.JSONResponse{
					Code: http.StatusMethodNotAllowed,
					JSON: spec.NotFound("unknown method"),
				}
			}),
		).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)
	}
	dendriteAdminRouter.Handle("/admin/registrationTokens/new",
		httputil.MakeAdminAPI("admin_registration_tokens_new", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return AdminCreateNewRegistrationToken(req, cfg, userAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	dendriteAdminRouter.Handle("/admin/registrationTokens",
		httputil.MakeAdminAPI("admin_list_registration_tokens", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return AdminListRegistrationTokens(req, cfg, userAPI)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	dendriteAdminRouter.Handle("/admin/registrationTokens/{token}",
		httputil.MakeAdminAPI("admin_get_registration_token", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			switch req.Method {
			case http.MethodGet:
				return AdminGetRegistrationToken(req, cfg, userAPI)
			case http.MethodPut:
				return AdminUpdateRegistrationToken(req, cfg, userAPI)
			case http.MethodDelete:
				return AdminDeleteRegistrationToken(req, cfg, userAPI)
			default:
				return util.MatrixErrorResponse(
					404,
					string(spec.ErrorNotFound),
					"unknown method",
				)
			}
		}),
	).Methods(http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodOptions)

	dendriteAdminRouter.Handle("/admin/evacuateRoom/{roomID}",
		httputil.MakeAdminAPI("admin_evacuate_room", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return AdminEvacuateRoom(req, rsAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	dendriteAdminRouter.Handle("/admin/evacuateUser/{userID}",
		httputil.MakeAdminAPI("admin_evacuate_user", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return AdminEvacuateUser(req, rsAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	dendriteAdminRouter.Handle("/admin/purgeRoom/{roomID}",
		httputil.MakeAdminAPI("admin_purge_room", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return AdminPurgeRoom(req, rsAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	dendriteAdminRouter.Handle("/admin/resetPassword/{userID}",
		httputil.MakeAdminAPI("admin_reset_password", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return AdminResetPassword(req, cfg, device, userAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	dendriteAdminRouter.Handle("/admin/downloadState/{serverName}/{roomID}",
		httputil.MakeAdminAPI("admin_download_state", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return AdminDownloadState(req, device, rsAPI)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	dendriteAdminRouter.Handle("/admin/fulltext/reindex",
		httputil.MakeAdminAPI("admin_fultext_reindex", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return AdminReindex(req, cfg, device, natsClient)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	dendriteAdminRouter.Handle("/admin/refreshDevices/{userID}",
		httputil.MakeAdminAPI("admin_refresh_devices", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return AdminMarkAsStale(req, cfg, userAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	// server notifications
	if cfg.Matrix.ServerNotices.Enabled {
		logrus.Info("Enabling server notices at /_synapse/admin/v1/send_server_notice")
		serverNotificationSender, err := getSenderDevice(context.Background(), rsAPI, userAPI, cfg)
		if err != nil {
			logrus.WithError(err).Fatal("unable to get account for sending sending server notices")
		}

		synapseAdminRouter.Handle("/admin/v1/send_server_notice/{txnID}",
			httputil.MakeAuthAPI("send_server_notice", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
				// not specced, but ensure we're rate limiting requests to this endpoint
				if r := rateLimits.Limit(req, device); r != nil {
					return *r
				}
				var vars map[string]string
				vars, err = httputil.URLDecodeMapValues(mux.Vars(req))
				if err != nil {
					return util.ErrorResponse(err)
				}
				txnID := vars["txnID"]
				return SendServerNotice(
					req, &cfg.Matrix.ServerNotices,
					cfg, userAPI, rsAPI, asAPI,
					device, serverNotificationSender,
					&txnID, transactionsCache,
				)
			}),
		).Methods(http.MethodPut, http.MethodOptions)

		synapseAdminRouter.Handle("/admin/v1/send_server_notice",
			httputil.MakeAuthAPI("send_server_notice", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
				// not specced, but ensure we're rate limiting requests to this endpoint
				if r := rateLimits.Limit(req, device); r != nil {
					return *r
				}
				return SendServerNotice(
					req, &cfg.Matrix.ServerNotices,
					cfg, userAPI, rsAPI, asAPI,
					device, serverNotificationSender,
					nil, transactionsCache,
				)
			}),
		).Methods(http.MethodPost, http.MethodOptions)
	}

	// You can't just do PathPrefix("/(r0|v3)") because regexps only apply when inside named path variables.
	// So make a named path variable called 'apiversion' (which we will never read in handlers) and then do
	// (r0|v3) - BUT this is a captured group, which makes no sense because you cannot extract this group
	// from a match (gorilla/mux exposes no way to do this) so it demands you make it a non-capturing group
	// using ?: so the final regexp becomes what is below. We also need a trailing slash to stop 'v33333' matching.
	// Note that 'apiversion' is chosen because it must not collide with a variable used in any of the routing!
	v3mux := publicAPIMux.PathPrefix("/{apiversion:(?:r0|v3)}/").Subrouter()

	v1mux := publicAPIMux.PathPrefix("/v1/").Subrouter()

	unstableMux := publicAPIMux.PathPrefix("/unstable").Subrouter()

	v3mux.Handle("/createRoom",
		httputil.MakeAuthAPI("createRoom", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return CreateRoom(req, device, cfg, userAPI, rsAPI, asAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/join/{roomIDOrAlias}",
		httputil.MakeAuthAPI(spec.Join, userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			// Only execute a join for roomIDOrAlias and UserID once. If there is a join in progress
			// it waits for it to complete and returns that result for subsequent requests.
			resp, _, _ := sf.Do(vars["roomIDOrAlias"]+device.UserID, func() (any, error) {
				return JoinRoomByIDOrAlias(
					req, device, rsAPI, userAPI, vars["roomIDOrAlias"],
				), nil
			})
			// once all joins are processed, drop them from the cache. Further requests
			// will be processed as usual.
			sf.Forget(vars["roomIDOrAlias"] + device.UserID)
			return resp.(util.JSONResponse)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodPost, http.MethodOptions)

	if mscCfg.Enabled("msc2753") {
		v3mux.Handle("/peek/{roomIDOrAlias}",
			httputil.MakeAuthAPI(spec.Peek, userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
				if r := rateLimits.Limit(req, device); r != nil {
					return *r
				}
				vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
				if err != nil {
					return util.ErrorResponse(err)
				}
				return PeekRoomByIDOrAlias(
					req, device, rsAPI, vars["roomIDOrAlias"],
				)
			}),
		).Methods(http.MethodPost, http.MethodOptions)
	}
	v3mux.Handle("/joined_rooms",
		httputil.MakeAuthAPI("joined_rooms", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return GetJoinedRooms(req, device, rsAPI)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)
	v3mux.Handle("/rooms/{roomID}/join",
		httputil.MakeAuthAPI(spec.Join, userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			// Only execute a join for roomID and UserID once. If there is a join in progress
			// it waits for it to complete and returns that result for subsequent requests.
			resp, _, _ := sf.Do(vars["roomID"]+device.UserID, func() (any, error) {
				return JoinRoomByIDOrAlias(
					req, device, rsAPI, userAPI, vars["roomID"],
				), nil
			})
			// once all joins are processed, drop them from the cache. Further requests
			// will be processed as usual.
			sf.Forget(vars["roomID"] + device.UserID)
			return resp.(util.JSONResponse)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/rooms/{roomID}/leave",
		httputil.MakeAuthAPI("membership", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return LeaveRoomByID(
				req, device, rsAPI, vars["roomID"],
			)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/rooms/{roomID}/unpeek",
		httputil.MakeAuthAPI("unpeek", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return UnpeekRoomByID(
				req, device, rsAPI, vars["roomID"],
			)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/rooms/{roomID}/ban",
		httputil.MakeAuthAPI("membership", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SendBan(req, userAPI, device, vars["roomID"], cfg, rsAPI, asAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/rooms/{roomID}/invite",
		httputil.MakeAuthAPI("membership", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SendInvite(req, userAPI, device, vars["roomID"], cfg, rsAPI, asAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/rooms/{roomID}/kick",
		httputil.MakeAuthAPI("membership", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SendKick(req, userAPI, device, vars["roomID"], cfg, rsAPI, asAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/rooms/{roomID}/unban",
		httputil.MakeAuthAPI("membership", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SendUnban(req, userAPI, device, vars["roomID"], cfg, rsAPI, asAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/rooms/{roomID}/send/{eventType}",
		httputil.MakeAuthAPI("send_message", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SendEvent(req, device, vars["roomID"], vars["eventType"], nil, nil, cfg, rsAPI, nil)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/rooms/{roomID}/send/{eventType}/{txnID}",
		httputil.MakeAuthAPI("send_message", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			txnID := vars["txnID"]
			return SendEvent(req, device, vars["roomID"], vars["eventType"], &txnID,
				nil, cfg, rsAPI, transactionsCache)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodPut, http.MethodOptions)

	v3mux.Handle("/rooms/{roomID}/state", httputil.MakeAuthAPI("room_state", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		return OnIncomingStateRequest(req.Context(), device, rsAPI, vars["roomID"])
	}, httputil.WithAllowGuests())).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/rooms/{roomID}/aliases", httputil.MakeAuthAPI("aliases", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		return GetAliases(req, rsAPI, device, vars["roomID"])
	})).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/rooms/{roomID}/state/{type:[^/]+/?}", httputil.MakeAuthAPI("room_state", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		// If there's a trailing slash, remove it
		eventType := strings.TrimSuffix(vars["type"], "/")
		eventFormat := req.URL.Query().Get("format") == "event"
		return OnIncomingStateTypeRequest(req.Context(), device, rsAPI, vars["roomID"], eventType, "", eventFormat)
	}, httputil.WithAllowGuests())).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/rooms/{roomID}/state/{type}/{stateKey}", httputil.MakeAuthAPI("room_state", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		eventFormat := req.URL.Query().Get("format") == "event"
		return OnIncomingStateTypeRequest(req.Context(), device, rsAPI, vars["roomID"], vars["type"], vars["stateKey"], eventFormat)
	}, httputil.WithAllowGuests())).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/rooms/{roomID}/state/{eventType:[^/]+/?}",
		httputil.MakeAuthAPI("send_message", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			emptyString := ""
			eventType := strings.TrimSuffix(vars["eventType"], "/")
			return SendEvent(req, device, vars["roomID"], eventType, nil, &emptyString, cfg, rsAPI, nil)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodPut, http.MethodOptions)

	v3mux.Handle("/rooms/{roomID}/state/{eventType}/{stateKey}",
		httputil.MakeAuthAPI("send_message", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			stateKey := vars["stateKey"]
			return SendEvent(req, device, vars["roomID"], vars["eventType"], nil, &stateKey, cfg, rsAPI, nil)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodPut, http.MethodOptions)

	// Defined outside of handler to persist between calls
	// TODO: clear based on some criteria
	roomHierarchyPaginationCache := NewRoomHierarchyPaginationCache()
	v1mux.Handle("/rooms/{roomID}/hierarchy",
		httputil.MakeAuthAPI("spaces", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return QueryRoomHierarchy(req, device, vars["roomID"], rsAPI, &roomHierarchyPaginationCache)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/register", httputil.MakeExternalAPI("register", func(req *http.Request) util.JSONResponse {
		if r := rateLimits.Limit(req, nil); r != nil {
			return *r
		}
		return Register(req, userAPI, cfg)
	})).Methods(http.MethodPost, http.MethodOptions)

	v3mux.Handle("/register/available", httputil.MakeExternalAPI("registerAvailable", func(req *http.Request) util.JSONResponse {
		if r := rateLimits.Limit(req, nil); r != nil {
			return *r
		}
		return RegisterAvailable(req, cfg, userAPI)
	})).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/directory/room/{roomAlias}",
		httputil.MakeExternalAPI("directory_room", func(req *http.Request) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return DirectoryRoom(req, vars["roomAlias"], federation, cfg, rsAPI, federationSender)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/directory/room/{roomAlias}",
		httputil.MakeAuthAPI("directory_room", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SetLocalAlias(req, device, vars["roomAlias"], cfg, rsAPI)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	v3mux.Handle("/directory/room/{roomAlias}",
		httputil.MakeAuthAPI("directory_room", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return RemoveLocalAlias(req, device, vars["roomAlias"], rsAPI)
		}),
	).Methods(http.MethodDelete, http.MethodOptions)
	v3mux.Handle("/directory/list/room/{roomID}",
		httputil.MakeExternalAPI("directory_list", func(req *http.Request) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetVisibility(req, rsAPI, vars["roomID"])
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/directory/list/room/{roomID}",
		httputil.MakeAuthAPI("directory_list", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SetVisibility(req, rsAPI, device, vars["roomID"])
		}),
	).Methods(http.MethodPut, http.MethodOptions)
	v3mux.Handle("/directory/list/appservice/{networkID}/{roomID}",
		httputil.MakeAuthAPI("directory_list", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SetVisibilityAS(req, rsAPI, device, vars["networkID"], vars["roomID"])
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	// Undocumented endpoint
	v3mux.Handle("/directory/list/appservice/{networkID}/{roomID}",
		httputil.MakeAuthAPI("directory_list", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SetVisibilityAS(req, rsAPI, device, vars["networkID"], vars["roomID"])
		}),
	).Methods(http.MethodDelete, http.MethodOptions)

	v3mux.Handle("/publicRooms",
		httputil.MakeExternalAPI("public_rooms", func(req *http.Request) util.JSONResponse {
			return GetPostPublicRooms(req, rsAPI, extRoomsProvider, federation, cfg)
		}),
	).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)

	v3mux.Handle("/logout",
		httputil.MakeAuthAPI("logout", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return Logout(req, userAPI, device)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	v3mux.Handle("/logout/all",
		httputil.MakeAuthAPI("logout", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return LogoutAll(req, userAPI, device)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	v3mux.Handle("/rooms/{roomID}/typing/{userID}",
		httputil.MakeAuthAPI("rooms_typing", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SendTyping(req, device, vars["roomID"], vars["userID"], rsAPI, syncProducer)
		}),
	).Methods(http.MethodPut, http.MethodOptions)
	v3mux.Handle("/rooms/{roomID}/redact/{eventID}",
		httputil.MakeAuthAPI("rooms_redact", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SendRedaction(req, device, vars["roomID"], vars["eventID"], cfg, rsAPI, nil, nil)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/rooms/{roomID}/redact/{eventID}/{txnId}",
		httputil.MakeAuthAPI("rooms_redact", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			txnID := vars["txnId"]
			return SendRedaction(req, device, vars["roomID"], vars["eventID"], cfg, rsAPI, &txnID, transactionsCache)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	v3mux.Handle("/sendToDevice/{eventType}/{txnID}",
		httputil.MakeAuthAPI("send_to_device", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			txnID := vars["txnID"]
			return SendToDevice(req, device, syncProducer, transactionsCache, vars["eventType"], &txnID)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodPut, http.MethodOptions)

	// This is only here because sytest refers to /unstable for this endpoint
	// rather than r0. It's an exact duplicate of the above handler.
	// TODO: Remove this if/when sytest is fixed!
	unstableMux.Handle("/sendToDevice/{eventType}/{txnID}",
		httputil.MakeAuthAPI("send_to_device", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			txnID := vars["txnID"]
			return SendToDevice(req, device, syncProducer, transactionsCache, vars["eventType"], &txnID)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodPut, http.MethodOptions)

	v3mux.Handle("/account/whoami",
		httputil.MakeAuthAPI("whoami", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			return Whoami(req, device)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/account/password",
		httputil.MakeAuthAPI("password", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			return Password(req, userAPI, device, cfg)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	v3mux.Handle("/account/deactivate",
		httputil.MakeAuthAPI("deactivate", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			return Deactivate(req, userInteractiveAuth, userAPI, device)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	// Stub endpoints required by Element

	v3mux.Handle("/login",
		httputil.MakeExternalAPI("login", func(req *http.Request) util.JSONResponse {
			if r := rateLimits.Limit(req, nil); r != nil {
				return *r
			}
			return Login(req, userAPI, cfg)
		}),
	).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)

	v3mux.Handle("/auth/{authType}/fallback/web",
		httputil.MakeHTMLAPI("auth_fallback", enableMetrics, func(w http.ResponseWriter, req *http.Request) {
			vars := mux.Vars(req)
			AuthFallback(w, req, vars["authType"], cfg)
		}),
	).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)

	// Push rules

	v3mux.Handle("/pushrules",
		httputil.MakeAuthAPI("push_rules", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam("missing trailing slash"),
			}
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/pushrules/",
		httputil.MakeAuthAPI("push_rules", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return GetAllPushRules(req.Context(), device, userAPI)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/pushrules/",
		httputil.MakeAuthAPI("push_rules", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam("scope, kind and rule ID must be specified"),
			}
		}),
	).Methods(http.MethodPut)

	v3mux.Handle("/pushrules/{scope}/",
		httputil.MakeAuthAPI("push_rules", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetPushRulesByScope(req.Context(), vars["scope"], device, userAPI)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/pushrules/{scope}",
		httputil.MakeAuthAPI("push_rules", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam("missing trailing slash after scope"),
			}
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/pushrules/{scope:[^/]+/?}",
		httputil.MakeAuthAPI("push_rules", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam("kind and rule ID must be specified"),
			}
		}),
	).Methods(http.MethodPut)

	v3mux.Handle("/pushrules/{scope}/{kind}/",
		httputil.MakeAuthAPI("push_rules", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetPushRulesByKind(req.Context(), vars["scope"], vars["kind"], device, userAPI)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/pushrules/{scope}/{kind}",
		httputil.MakeAuthAPI("push_rules", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam("missing trailing slash after kind"),
			}
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/pushrules/{scope}/{kind:[^/]+/?}",
		httputil.MakeAuthAPI("push_rules", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam("rule ID must be specified"),
			}
		}),
	).Methods(http.MethodPut)

	v3mux.Handle("/pushrules/{scope}/{kind}/{ruleID}",
		httputil.MakeAuthAPI("push_rules", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetPushRuleByRuleID(req.Context(), vars["scope"], vars["kind"], vars["ruleID"], device, userAPI)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/pushrules/{scope}/{kind}/{ruleID}",
		httputil.MakeAuthAPI("push_rules", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			query := req.URL.Query()
			return PutPushRuleByRuleID(req.Context(), vars["scope"], vars["kind"], vars["ruleID"], query.Get("after"), query.Get("before"), req.Body, device, userAPI)
		}),
	).Methods(http.MethodPut)

	v3mux.Handle("/pushrules/{scope}/{kind}/{ruleID}",
		httputil.MakeAuthAPI("push_rules", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return DeletePushRuleByRuleID(req.Context(), vars["scope"], vars["kind"], vars["ruleID"], device, userAPI)
		}),
	).Methods(http.MethodDelete)

	v3mux.Handle("/pushrules/{scope}/{kind}/{ruleID}/{attr}",
		httputil.MakeAuthAPI("push_rules", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetPushRuleAttrByRuleID(req.Context(), vars["scope"], vars["kind"], vars["ruleID"], vars["attr"], device, userAPI)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/pushrules/{scope}/{kind}/{ruleID}/{attr}",
		httputil.MakeAuthAPI("push_rules", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return PutPushRuleAttrByRuleID(req.Context(), vars["scope"], vars["kind"], vars["ruleID"], vars["attr"], req.Body, device, userAPI)
		}),
	).Methods(http.MethodPut)

	// Element user settings

	v3mux.Handle("/profile/{userID}",
		httputil.MakeExternalAPI("profile", func(req *http.Request) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetProfile(req, userAPI, cfg, vars["userID"], asAPI, federation)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/profile/{userID}/avatar_url",
		httputil.MakeExternalAPI("profile_avatar_url", func(req *http.Request) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetAvatarURL(req, userAPI, cfg, vars["userID"], asAPI, federation)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/profile/{userID}/avatar_url",
		httputil.MakeAuthAPI("profile_avatar_url", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SetAvatarURL(req, userAPI, device, vars["userID"], cfg, rsAPI)
		}),
	).Methods(http.MethodPut, http.MethodOptions)
	// Browsers use the OPTIONS HTTP method to check if the CORS policy allows
	// PUT requests, so we need to allow this method

	v3mux.Handle("/profile/{userID}/displayname",
		httputil.MakeExternalAPI("profile_displayname", func(req *http.Request) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetDisplayName(req, userAPI, cfg, vars["userID"], asAPI, federation)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/profile/{userID}/displayname",
		httputil.MakeAuthAPI("profile_displayname", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SetDisplayName(req, userAPI, device, vars["userID"], cfg, rsAPI)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodPut, http.MethodOptions)
	// Browsers use the OPTIONS HTTP method to check if the CORS policy allows
	// PUT requests, so we need to allow this method

	threePIDClient := base.CreateClient(dendriteCfg, nil) // TODO: Move this somewhere else, e.g. pass in as parameter

	v3mux.Handle("/account/3pid",
		httputil.MakeAuthAPI("account_3pid", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return GetAssociated3PIDs(req, userAPI, device)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/account/3pid",
		httputil.MakeAuthAPI("account_3pid", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return CheckAndSave3PIDAssociation(req, userAPI, device, cfg, threePIDClient)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	v3mux.Handle("/account/3pid/delete",
		httputil.MakeAuthAPI("account_3pid", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return Forget3PID(req, userAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	v3mux.Handle("/{path:(?:account/3pid|register)}/email/requestToken",
		httputil.MakeExternalAPI("account_3pid_request_token", func(req *http.Request) util.JSONResponse {
			return RequestEmailToken(req, userAPI, cfg, threePIDClient)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	v3mux.Handle("/voip/turnServer",
		httputil.MakeAuthAPI("turn_server", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			return RequestTurnServer(req, device, cfg)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/thirdparty/protocols",
		httputil.MakeAuthAPI("thirdparty_protocols", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return Protocols(req, asAPI, device, "")
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/thirdparty/protocol/{protocolID}",
		httputil.MakeAuthAPI("thirdparty_protocols", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return Protocols(req, asAPI, device, vars["protocolID"])
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/thirdparty/user/{protocolID}",
		httputil.MakeAuthAPI("thirdparty_user", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return User(req, asAPI, device, vars["protocolID"], req.URL.Query())
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/thirdparty/user",
		httputil.MakeAuthAPI("thirdparty_user", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return User(req, asAPI, device, "", req.URL.Query())
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/thirdparty/location/{protocolID}",
		httputil.MakeAuthAPI("thirdparty_location", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return Location(req, asAPI, device, vars["protocolID"], req.URL.Query())
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/thirdparty/location",
		httputil.MakeAuthAPI("thirdparty_location", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return Location(req, asAPI, device, "", req.URL.Query())
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/rooms/{roomID}/initialSync",
		httputil.MakeExternalAPI("rooms_initial_sync", func(req *http.Request) util.JSONResponse {
			// TODO: Allow people to peek into rooms.
			return util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: spec.GuestAccessForbidden("Guest access not implemented"),
			}
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/user/{userID}/account_data/{type}",
		httputil.MakeAuthAPI("user_account_data", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SaveAccountData(req, userAPI, device, vars["userID"], "", vars["type"], syncProducer)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	v3mux.Handle("/user/{userID}/rooms/{roomID}/account_data/{type}",
		httputil.MakeAuthAPI("user_account_data", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SaveAccountData(req, userAPI, device, vars["userID"], vars["roomID"], vars["type"], syncProducer)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	v3mux.Handle("/user/{userID}/account_data/{type}",
		httputil.MakeAuthAPI("user_account_data", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetAccountData(req, userAPI, device, vars["userID"], "", vars["type"])
		}),
	).Methods(http.MethodGet)

	v3mux.Handle("/user/{userID}/rooms/{roomID}/account_data/{type}",
		httputil.MakeAuthAPI("user_account_data", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetAccountData(req, userAPI, device, vars["userID"], vars["roomID"], vars["type"])
		}),
	).Methods(http.MethodGet)

	v3mux.Handle("/admin/whois/{userID}",
		httputil.MakeAuthAPI("admin_whois", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetAdminWhois(req, userAPI, device, vars["userID"])
		}),
	).Methods(http.MethodGet)

	v3mux.Handle("/user/{userID}/openid/request_token",
		httputil.MakeAuthAPI("openid_request_token", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return CreateOpenIDToken(req, userAPI, device, vars["userID"], cfg)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	v3mux.Handle("/user_directory/search",
		httputil.MakeAuthAPI("userdirectory_search", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			postContent := struct {
				SearchString string `json:"search_term"`
				Limit        int    `json:"limit"`
			}{}

			if resErr := clientutil.UnmarshalJSONRequest(req, &postContent); resErr != nil {
				return *resErr
			}
			return SearchUserDirectory(
				req.Context(),
				device,
				rsAPI,
				userDirectoryProvider,
				postContent.SearchString,
				postContent.Limit,
				federation,
				cfg.Matrix.ServerName,
			)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	v3mux.Handle("/rooms/{roomID}/read_markers",
		httputil.MakeAuthAPI("rooms_read_markers", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SaveReadMarker(req, userAPI, rsAPI, syncProducer, device, vars["roomID"])
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	v3mux.Handle("/rooms/{roomID}/forget",
		httputil.MakeAuthAPI("rooms_forget", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SendForget(req, device, vars["roomID"], rsAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	v3mux.Handle("/rooms/{roomID}/upgrade",
		httputil.MakeAuthAPI("rooms_upgrade", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return UpgradeRoom(req, device, cfg, vars["roomID"], userAPI, rsAPI, asAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	v3mux.Handle("/devices",
		httputil.MakeAuthAPI("get_devices", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return GetDevicesByLocalpart(req, userAPI, device)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/devices/{deviceID}",
		httputil.MakeAuthAPI("get_device", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetDeviceByID(req, userAPI, device, vars["deviceID"])
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/devices/{deviceID}",
		httputil.MakeAuthAPI("device_data", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return UpdateDeviceByID(req, userAPI, device, vars["deviceID"])
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodPut, http.MethodOptions)

	v3mux.Handle("/devices/{deviceID}",
		httputil.MakeAuthAPI("delete_device", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return DeleteDeviceById(req, userInteractiveAuth, userAPI, device, vars["deviceID"])
		}),
	).Methods(http.MethodDelete, http.MethodOptions)

	v3mux.Handle("/delete_devices",
		httputil.MakeAuthAPI("delete_devices", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return DeleteDevices(req, userInteractiveAuth, userAPI, device)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	v3mux.Handle("/notifications",
		httputil.MakeAuthAPI("get_notifications", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return GetNotifications(req, device, userAPI)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/pushers",
		httputil.MakeAuthAPI("get_pushers", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return GetPushers(req, device, userAPI)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/pushers/set",
		httputil.MakeAuthAPI("set_pushers", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			return SetPusher(req, device, userAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	// Stub implementations for sytest
	v3mux.Handle("/events",
		httputil.MakeAuthAPI("events", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return util.JSONResponse{Code: http.StatusOK, JSON: map[string]interface{}{
				"chunk": []interface{}{},
				"start": "",
				"end":   "",
			}}
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/initialSync",
		httputil.MakeAuthAPI("initial_sync", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return util.JSONResponse{Code: http.StatusOK, JSON: map[string]interface{}{
				"end": "",
			}}
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/user/{userId}/rooms/{roomId}/tags",
		httputil.MakeAuthAPI("get_tags", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetTags(req, userAPI, device, vars["userId"], vars["roomId"], syncProducer)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	v3mux.Handle("/user/{userId}/rooms/{roomId}/tags/{tag}",
		httputil.MakeAuthAPI("put_tag", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return PutTag(req, userAPI, device, vars["userId"], vars["roomId"], vars["tag"], syncProducer)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	v3mux.Handle("/user/{userId}/rooms/{roomId}/tags/{tag}",
		httputil.MakeAuthAPI("delete_tag", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return DeleteTag(req, userAPI, device, vars["userId"], vars["roomId"], vars["tag"], syncProducer)
		}),
	).Methods(http.MethodDelete, http.MethodOptions)

	v3mux.Handle("/capabilities",
		httputil.MakeAuthAPI("capabilities", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			return GetCapabilities()
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodGet, http.MethodOptions)

	// Key Backup Versions (Metadata)

	getBackupKeysVersion := httputil.MakeAuthAPI("get_backup_keys_version", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		return KeyBackupVersion(req, userAPI, device, vars["version"])
	})

	getLatestBackupKeysVersion := httputil.MakeAuthAPI("get_latest_backup_keys_version", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		return KeyBackupVersion(req, userAPI, device, "")
	})

	putBackupKeysVersion := httputil.MakeAuthAPI("put_backup_keys_version", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		return ModifyKeyBackupVersionAuthData(req, userAPI, device, vars["version"])
	})

	deleteBackupKeysVersion := httputil.MakeAuthAPI("delete_backup_keys_version", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		return DeleteKeyBackupVersion(req, userAPI, device, vars["version"])
	})

	postNewBackupKeysVersion := httputil.MakeAuthAPI("post_new_backup_keys_version", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		return CreateKeyBackupVersion(req, userAPI, device)
	})

	v3mux.Handle("/room_keys/version/{version}", getBackupKeysVersion).Methods(http.MethodGet, http.MethodOptions)
	v3mux.Handle("/room_keys/version", getLatestBackupKeysVersion).Methods(http.MethodGet, http.MethodOptions)
	v3mux.Handle("/room_keys/version/{version}", putBackupKeysVersion).Methods(http.MethodPut)
	v3mux.Handle("/room_keys/version/{version}", deleteBackupKeysVersion).Methods(http.MethodDelete)
	v3mux.Handle("/room_keys/version", postNewBackupKeysVersion).Methods(http.MethodPost, http.MethodOptions)

	unstableMux.Handle("/room_keys/version/{version}", getBackupKeysVersion).Methods(http.MethodGet, http.MethodOptions)
	unstableMux.Handle("/room_keys/version", getLatestBackupKeysVersion).Methods(http.MethodGet, http.MethodOptions)
	unstableMux.Handle("/room_keys/version/{version}", putBackupKeysVersion).Methods(http.MethodPut)
	unstableMux.Handle("/room_keys/version/{version}", deleteBackupKeysVersion).Methods(http.MethodDelete)
	unstableMux.Handle("/room_keys/version", postNewBackupKeysVersion).Methods(http.MethodPost, http.MethodOptions)

	// Inserting E2E Backup Keys

	// Bulk room and session
	putBackupKeys := httputil.MakeAuthAPI("put_backup_keys", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		version := req.URL.Query().Get("version")
		if version == "" {
			return util.JSONResponse{
				Code: 400,
				JSON: spec.InvalidParam("version must be specified"),
			}
		}
		var reqBody keyBackupSessionRequest
		resErr := clientutil.UnmarshalJSONRequest(req, &reqBody)
		if resErr != nil {
			return *resErr
		}
		return UploadBackupKeys(req, userAPI, device, version, &reqBody)
	})

	// Single room bulk session
	putBackupKeysRoom := httputil.MakeAuthAPI("put_backup_keys_room", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		version := req.URL.Query().Get("version")
		if version == "" {
			return util.JSONResponse{
				Code: 400,
				JSON: spec.InvalidParam("version must be specified"),
			}
		}
		roomID := vars["roomID"]
		var reqBody keyBackupSessionRequest
		reqBody.Rooms = make(map[string]struct {
			Sessions map[string]userapi.KeyBackupSession `json:"sessions"`
		})
		reqBody.Rooms[roomID] = struct {
			Sessions map[string]userapi.KeyBackupSession `json:"sessions"`
		}{
			Sessions: map[string]userapi.KeyBackupSession{},
		}
		body := reqBody.Rooms[roomID]
		resErr := clientutil.UnmarshalJSONRequest(req, &body)
		if resErr != nil {
			return *resErr
		}
		reqBody.Rooms[roomID] = body
		return UploadBackupKeys(req, userAPI, device, version, &reqBody)
	})

	// Single room, single session
	putBackupKeysRoomSession := httputil.MakeAuthAPI("put_backup_keys_room_session", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		version := req.URL.Query().Get("version")
		if version == "" {
			return util.JSONResponse{
				Code: 400,
				JSON: spec.InvalidParam("version must be specified"),
			}
		}
		var reqBody userapi.KeyBackupSession
		resErr := clientutil.UnmarshalJSONRequest(req, &reqBody)
		if resErr != nil {
			return *resErr
		}
		roomID := vars["roomID"]
		sessionID := vars["sessionID"]
		var keyReq keyBackupSessionRequest
		keyReq.Rooms = make(map[string]struct {
			Sessions map[string]userapi.KeyBackupSession `json:"sessions"`
		})
		keyReq.Rooms[roomID] = struct {
			Sessions map[string]userapi.KeyBackupSession `json:"sessions"`
		}{
			Sessions: make(map[string]userapi.KeyBackupSession),
		}
		keyReq.Rooms[roomID].Sessions[sessionID] = reqBody
		return UploadBackupKeys(req, userAPI, device, version, &keyReq)
	})

	v3mux.Handle("/room_keys/keys", putBackupKeys).Methods(http.MethodPut)
	v3mux.Handle("/room_keys/keys/{roomID}", putBackupKeysRoom).Methods(http.MethodPut)
	v3mux.Handle("/room_keys/keys/{roomID}/{sessionID}", putBackupKeysRoomSession).Methods(http.MethodPut)

	unstableMux.Handle("/room_keys/keys", putBackupKeys).Methods(http.MethodPut)
	unstableMux.Handle("/room_keys/keys/{roomID}", putBackupKeysRoom).Methods(http.MethodPut)
	unstableMux.Handle("/room_keys/keys/{roomID}/{sessionID}", putBackupKeysRoomSession).Methods(http.MethodPut)

	// Querying E2E Backup Keys

	getBackupKeys := httputil.MakeAuthAPI("get_backup_keys", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		return GetBackupKeys(req, userAPI, device, req.URL.Query().Get("version"), "", "")
	})

	getBackupKeysRoom := httputil.MakeAuthAPI("get_backup_keys_room", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		return GetBackupKeys(req, userAPI, device, req.URL.Query().Get("version"), vars["roomID"], "")
	})

	getBackupKeysRoomSession := httputil.MakeAuthAPI("get_backup_keys_room_session", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		return GetBackupKeys(req, userAPI, device, req.URL.Query().Get("version"), vars["roomID"], vars["sessionID"])
	})

	v3mux.Handle("/room_keys/keys", getBackupKeys).Methods(http.MethodGet, http.MethodOptions)
	v3mux.Handle("/room_keys/keys/{roomID}", getBackupKeysRoom).Methods(http.MethodGet, http.MethodOptions)
	v3mux.Handle("/room_keys/keys/{roomID}/{sessionID}", getBackupKeysRoomSession).Methods(http.MethodGet, http.MethodOptions)

	unstableMux.Handle("/room_keys/keys", getBackupKeys).Methods(http.MethodGet, http.MethodOptions)
	unstableMux.Handle("/room_keys/keys/{roomID}", getBackupKeysRoom).Methods(http.MethodGet, http.MethodOptions)
	unstableMux.Handle("/room_keys/keys/{roomID}/{sessionID}", getBackupKeysRoomSession).Methods(http.MethodGet, http.MethodOptions)

	// Deleting E2E Backup Keys

	// Cross-signing device keys

	postDeviceSigningKeys := httputil.MakeAuthAPI("post_device_signing_keys", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		return UploadCrossSigningDeviceKeys(req, userInteractiveAuth, userAPI, device, userAPI, cfg)
	})

	postDeviceSigningSignatures := httputil.MakeAuthAPI("post_device_signing_signatures", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
		return UploadCrossSigningDeviceSignatures(req, userAPI, device)
	}, httputil.WithAllowGuests())

	v3mux.Handle("/keys/device_signing/upload", postDeviceSigningKeys).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/keys/signatures/upload", postDeviceSigningSignatures).Methods(http.MethodPost, http.MethodOptions)

	unstableMux.Handle("/keys/device_signing/upload", postDeviceSigningKeys).Methods(http.MethodPost, http.MethodOptions)
	unstableMux.Handle("/keys/signatures/upload", postDeviceSigningSignatures).Methods(http.MethodPost, http.MethodOptions)

	// Supplying a device ID is deprecated.
	v3mux.Handle("/keys/upload/{deviceID}",
		httputil.MakeAuthAPI("keys_upload", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return UploadKeys(req, userAPI, device)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/keys/upload",
		httputil.MakeAuthAPI("keys_upload", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return UploadKeys(req, userAPI, device)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/keys/query",
		httputil.MakeAuthAPI("keys_query", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return QueryKeys(req, userAPI, device)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/keys/claim",
		httputil.MakeAuthAPI("keys_claim", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			return ClaimKeys(req, userAPI)
		}, httputil.WithAllowGuests()),
	).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/rooms/{roomId}/receipt/{receiptType}/{eventId}",
		httputil.MakeAuthAPI(spec.Join, userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			if r := rateLimits.Limit(req, device); r != nil {
				return *r
			}
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}

			return SetReceipt(req, userAPI, syncProducer, device, vars["roomId"], vars["receiptType"], vars["eventId"])
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	v3mux.Handle("/presence/{userId}/status",
		httputil.MakeAuthAPI("set_presence", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SetPresence(req, cfg, device, syncProducer, vars["userId"])
		}),
	).Methods(http.MethodPut, http.MethodOptions)
	v3mux.Handle("/presence/{userId}/status",
		httputil.MakeAuthAPI("get_presence", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetPresence(req, device, natsClient, cfg.Matrix.JetStream.Prefixed(jetstream.RequestPresence), vars["userId"])
		}),
	).Methods(http.MethodGet, http.MethodOptions)
}
