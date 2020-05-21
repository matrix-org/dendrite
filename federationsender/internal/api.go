package internal

import (
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/federationsender/producers"
	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// FederationSenderInternalAPI is an implementation of api.FederationSenderInternalAPI
type FederationSenderInternalAPI struct {
	api.FederationSenderInternalAPI
	db         storage.Database
	cfg        *config.Dendrite
	statistics *types.Statistics
	producer   *producers.RoomserverProducer
	federation *gomatrixserverlib.FederationClient
	keyRing    *gomatrixserverlib.KeyRing
}

func NewFederationSenderInternalAPI(
	db storage.Database, cfg *config.Dendrite,
	producer *producers.RoomserverProducer,
	federation *gomatrixserverlib.FederationClient,
	keyRing *gomatrixserverlib.KeyRing,
	statistics *types.Statistics,
) *FederationSenderInternalAPI {
	return &FederationSenderInternalAPI{
		db:         db,
		cfg:        cfg,
		producer:   producer,
		federation: federation,
		keyRing:    keyRing,
		statistics: statistics,
	}
}

// SetupHTTP adds the FederationSenderInternalAPI handlers to the http.ServeMux.
func (f *FederationSenderInternalAPI) SetupHTTP(servMux *http.ServeMux) {
	servMux.Handle(
		api.FederationSenderQueryJoinedHostsInRoomPath,
		internal.MakeInternalAPI("QueryJoinedHostsInRoom", func(req *http.Request) util.JSONResponse {
			var request api.QueryJoinedHostsInRoomRequest
			var response api.QueryJoinedHostsInRoomResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := f.QueryJoinedHostsInRoom(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		api.FederationSenderQueryJoinedHostServerNamesInRoomPath,
		internal.MakeInternalAPI("QueryJoinedHostServerNamesInRoom", func(req *http.Request) util.JSONResponse {
			var request api.QueryJoinedHostServerNamesInRoomRequest
			var response api.QueryJoinedHostServerNamesInRoomResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := f.QueryJoinedHostServerNamesInRoom(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(api.FederationSenderPerformJoinRequestPath,
		internal.MakeInternalAPI("PerformJoinRequest", func(req *http.Request) util.JSONResponse {
			var request api.PerformJoinRequest
			var response api.PerformJoinResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := f.PerformJoin(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(api.FederationSenderPerformLeaveRequestPath,
		internal.MakeInternalAPI("PerformLeaveRequest", func(req *http.Request) util.JSONResponse {
			var request api.PerformLeaveRequest
			var response api.PerformLeaveResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := f.PerformLeave(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
