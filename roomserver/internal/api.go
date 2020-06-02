package internal

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/Shopify/sarama"
	"github.com/gorilla/mux"
	fsAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// RoomserverInternalAPI is an implementation of api.RoomserverInternalAPI
type RoomserverInternalAPI struct {
	DB                   storage.Database
	Cfg                  *config.Dendrite
	Producer             sarama.SyncProducer
	ImmutableCache       caching.ImmutableCache
	ServerName           gomatrixserverlib.ServerName
	KeyRing              gomatrixserverlib.JSONVerifier
	FedClient            *gomatrixserverlib.FederationClient
	OutputRoomEventTopic string     // Kafka topic for new output room events
	mutex                sync.Mutex // Protects calls to processRoomEvent
	fsAPI                fsAPI.FederationSenderInternalAPI
}

// SetupHTTP adds the RoomserverInternalAPI handlers to the http.ServeMux.
// nolint: gocyclo
func (r *RoomserverInternalAPI) SetupHTTP(internalAPIMux *mux.Router) {
	internalAPIMux.Handle(api.RoomserverInputRoomEventsPath,
		internal.MakeInternalAPI("inputRoomEvents", func(req *http.Request) util.JSONResponse {
			var request api.InputRoomEventsRequest
			var response api.InputRoomEventsResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.InputRoomEvents(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(api.RoomserverPerformJoinPath,
		internal.MakeInternalAPI("performJoin", func(req *http.Request) util.JSONResponse {
			var request api.PerformJoinRequest
			var response api.PerformJoinResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.PerformJoin(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(api.RoomserverPerformLeavePath,
		internal.MakeInternalAPI("performLeave", func(req *http.Request) util.JSONResponse {
			var request api.PerformLeaveRequest
			var response api.PerformLeaveResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.PerformLeave(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverQueryLatestEventsAndStatePath,
		internal.MakeInternalAPI("queryLatestEventsAndState", func(req *http.Request) util.JSONResponse {
			var request api.QueryLatestEventsAndStateRequest
			var response api.QueryLatestEventsAndStateResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryLatestEventsAndState(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverQueryStateAfterEventsPath,
		internal.MakeInternalAPI("queryStateAfterEvents", func(req *http.Request) util.JSONResponse {
			var request api.QueryStateAfterEventsRequest
			var response api.QueryStateAfterEventsResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryStateAfterEvents(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverQueryEventsByIDPath,
		internal.MakeInternalAPI("queryEventsByID", func(req *http.Request) util.JSONResponse {
			var request api.QueryEventsByIDRequest
			var response api.QueryEventsByIDResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryEventsByID(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverQueryMembershipForUserPath,
		internal.MakeInternalAPI("QueryMembershipForUser", func(req *http.Request) util.JSONResponse {
			var request api.QueryMembershipForUserRequest
			var response api.QueryMembershipForUserResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryMembershipForUser(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverQueryMembershipsForRoomPath,
		internal.MakeInternalAPI("queryMembershipsForRoom", func(req *http.Request) util.JSONResponse {
			var request api.QueryMembershipsForRoomRequest
			var response api.QueryMembershipsForRoomResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryMembershipsForRoom(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverQueryInvitesForUserPath,
		internal.MakeInternalAPI("queryInvitesForUser", func(req *http.Request) util.JSONResponse {
			var request api.QueryInvitesForUserRequest
			var response api.QueryInvitesForUserResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryInvitesForUser(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverQueryServerAllowedToSeeEventPath,
		internal.MakeInternalAPI("queryServerAllowedToSeeEvent", func(req *http.Request) util.JSONResponse {
			var request api.QueryServerAllowedToSeeEventRequest
			var response api.QueryServerAllowedToSeeEventResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryServerAllowedToSeeEvent(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverQueryMissingEventsPath,
		internal.MakeInternalAPI("queryMissingEvents", func(req *http.Request) util.JSONResponse {
			var request api.QueryMissingEventsRequest
			var response api.QueryMissingEventsResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryMissingEvents(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverQueryStateAndAuthChainPath,
		internal.MakeInternalAPI("queryStateAndAuthChain", func(req *http.Request) util.JSONResponse {
			var request api.QueryStateAndAuthChainRequest
			var response api.QueryStateAndAuthChainResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryStateAndAuthChain(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverQueryBackfillPath,
		internal.MakeInternalAPI("QueryBackfill", func(req *http.Request) util.JSONResponse {
			var request api.QueryBackfillRequest
			var response api.QueryBackfillResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryBackfill(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverQueryRoomVersionCapabilitiesPath,
		internal.MakeInternalAPI("QueryRoomVersionCapabilities", func(req *http.Request) util.JSONResponse {
			var request api.QueryRoomVersionCapabilitiesRequest
			var response api.QueryRoomVersionCapabilitiesResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryRoomVersionCapabilities(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverQueryRoomVersionForRoomPath,
		internal.MakeInternalAPI("QueryRoomVersionForRoom", func(req *http.Request) util.JSONResponse {
			var request api.QueryRoomVersionForRoomRequest
			var response api.QueryRoomVersionForRoomResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryRoomVersionForRoom(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverSetRoomAliasPath,
		internal.MakeInternalAPI("setRoomAlias", func(req *http.Request) util.JSONResponse {
			var request api.SetRoomAliasRequest
			var response api.SetRoomAliasResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.SetRoomAlias(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverGetRoomIDForAliasPath,
		internal.MakeInternalAPI("GetRoomIDForAlias", func(req *http.Request) util.JSONResponse {
			var request api.GetRoomIDForAliasRequest
			var response api.GetRoomIDForAliasResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.GetRoomIDForAlias(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverGetCreatorIDForAliasPath,
		internal.MakeInternalAPI("GetCreatorIDForAlias", func(req *http.Request) util.JSONResponse {
			var request api.GetCreatorIDForAliasRequest
			var response api.GetCreatorIDForAliasResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.GetCreatorIDForAlias(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverGetAliasesForRoomIDPath,
		internal.MakeInternalAPI("getAliasesForRoomID", func(req *http.Request) util.JSONResponse {
			var request api.GetAliasesForRoomIDRequest
			var response api.GetAliasesForRoomIDResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.GetAliasesForRoomID(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		api.RoomserverRemoveRoomAliasPath,
		internal.MakeInternalAPI("removeRoomAlias", func(req *http.Request) util.JSONResponse {
			var request api.RemoveRoomAliasRequest
			var response api.RemoveRoomAliasResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.RemoveRoomAlias(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
