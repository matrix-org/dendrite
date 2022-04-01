package inthttp

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/util"
)

// AddRoutes adds the RoomserverInternalAPI handlers to the http.ServeMux.
// nolint: gocyclo
func AddRoutes(r api.RoomserverInternalAPI, internalAPIMux *mux.Router) {
	internalAPIMux.Handle(RoomserverInputRoomEventsPath,
		httputil.MakeInternalAPI("inputRoomEvents", func(req *http.Request) util.JSONResponse {
			var request api.InputRoomEventsRequest
			var response api.InputRoomEventsResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			r.InputRoomEvents(req.Context(), &request, &response)
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(RoomserverPerformInvitePath,
		httputil.MakeInternalAPI("performInvite", func(req *http.Request) util.JSONResponse {
			var request api.PerformInviteRequest
			var response api.PerformInviteResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.PerformInvite(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(RoomserverPerformJoinPath,
		httputil.MakeInternalAPI("performJoin", func(req *http.Request) util.JSONResponse {
			var request api.PerformJoinRequest
			var response api.PerformJoinResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			r.PerformJoin(req.Context(), &request, &response)
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(RoomserverPerformLeavePath,
		httputil.MakeInternalAPI("performLeave", func(req *http.Request) util.JSONResponse {
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
	internalAPIMux.Handle(RoomserverPerformPeekPath,
		httputil.MakeInternalAPI("performPeek", func(req *http.Request) util.JSONResponse {
			var request api.PerformPeekRequest
			var response api.PerformPeekResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			r.PerformPeek(req.Context(), &request, &response)
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(RoomserverPerformInboundPeekPath,
		httputil.MakeInternalAPI("performInboundPeek", func(req *http.Request) util.JSONResponse {
			var request api.PerformInboundPeekRequest
			var response api.PerformInboundPeekResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.PerformInboundPeek(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(RoomserverPerformPeekPath,
		httputil.MakeInternalAPI("performUnpeek", func(req *http.Request) util.JSONResponse {
			var request api.PerformUnpeekRequest
			var response api.PerformUnpeekResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			r.PerformUnpeek(req.Context(), &request, &response)
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(RoomserverPerformRoomUpgradePath,
		httputil.MakeInternalAPI("performRoomUpgrade", func(req *http.Request) util.JSONResponse {
			var request api.PerformRoomUpgradeRequest
			var response api.PerformRoomUpgradeResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			r.PerformRoomUpgrade(req.Context(), &request, &response)
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(RoomserverPerformPublishPath,
		httputil.MakeInternalAPI("performPublish", func(req *http.Request) util.JSONResponse {
			var request api.PerformPublishRequest
			var response api.PerformPublishResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			r.PerformPublish(req.Context(), &request, &response)
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		RoomserverQueryPublishedRoomsPath,
		httputil.MakeInternalAPI("queryPublishedRooms", func(req *http.Request) util.JSONResponse {
			var request api.QueryPublishedRoomsRequest
			var response api.QueryPublishedRoomsResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryPublishedRooms(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		RoomserverQueryLatestEventsAndStatePath,
		httputil.MakeInternalAPI("queryLatestEventsAndState", func(req *http.Request) util.JSONResponse {
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
		RoomserverQueryStateAfterEventsPath,
		httputil.MakeInternalAPI("queryStateAfterEvents", func(req *http.Request) util.JSONResponse {
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
		RoomserverQueryEventsByIDPath,
		httputil.MakeInternalAPI("queryEventsByID", func(req *http.Request) util.JSONResponse {
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
		RoomserverQueryMembershipForUserPath,
		httputil.MakeInternalAPI("QueryMembershipForUser", func(req *http.Request) util.JSONResponse {
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
		RoomserverQueryMembershipsForRoomPath,
		httputil.MakeInternalAPI("queryMembershipsForRoom", func(req *http.Request) util.JSONResponse {
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
		RoomserverQueryServerJoinedToRoomPath,
		httputil.MakeInternalAPI("queryServerJoinedToRoom", func(req *http.Request) util.JSONResponse {
			var request api.QueryServerJoinedToRoomRequest
			var response api.QueryServerJoinedToRoomResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryServerJoinedToRoom(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		RoomserverQueryServerAllowedToSeeEventPath,
		httputil.MakeInternalAPI("queryServerAllowedToSeeEvent", func(req *http.Request) util.JSONResponse {
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
		RoomserverQueryMissingEventsPath,
		httputil.MakeInternalAPI("queryMissingEvents", func(req *http.Request) util.JSONResponse {
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
		RoomserverQueryStateAndAuthChainPath,
		httputil.MakeInternalAPI("queryStateAndAuthChain", func(req *http.Request) util.JSONResponse {
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
		RoomserverPerformBackfillPath,
		httputil.MakeInternalAPI("PerformBackfill", func(req *http.Request) util.JSONResponse {
			var request api.PerformBackfillRequest
			var response api.PerformBackfillResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.PerformBackfill(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		RoomserverPerformForgetPath,
		httputil.MakeInternalAPI("PerformForget", func(req *http.Request) util.JSONResponse {
			var request api.PerformForgetRequest
			var response api.PerformForgetResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.PerformForget(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		RoomserverQueryRoomVersionCapabilitiesPath,
		httputil.MakeInternalAPI("QueryRoomVersionCapabilities", func(req *http.Request) util.JSONResponse {
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
		RoomserverQueryRoomVersionForRoomPath,
		httputil.MakeInternalAPI("QueryRoomVersionForRoom", func(req *http.Request) util.JSONResponse {
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
		RoomserverSetRoomAliasPath,
		httputil.MakeInternalAPI("setRoomAlias", func(req *http.Request) util.JSONResponse {
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
		RoomserverGetRoomIDForAliasPath,
		httputil.MakeInternalAPI("GetRoomIDForAlias", func(req *http.Request) util.JSONResponse {
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
		RoomserverGetCreatorIDForAliasPath,
		httputil.MakeInternalAPI("GetCreatorIDForAlias", func(req *http.Request) util.JSONResponse {
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
		RoomserverGetAliasesForRoomIDPath,
		httputil.MakeInternalAPI("getAliasesForRoomID", func(req *http.Request) util.JSONResponse {
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
		RoomserverRemoveRoomAliasPath,
		httputil.MakeInternalAPI("removeRoomAlias", func(req *http.Request) util.JSONResponse {
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
	internalAPIMux.Handle(RoomserverQueryCurrentStatePath,
		httputil.MakeInternalAPI("queryCurrentState", func(req *http.Request) util.JSONResponse {
			request := api.QueryCurrentStateRequest{}
			response := api.QueryCurrentStateResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.QueryCurrentState(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(RoomserverQueryRoomsForUserPath,
		httputil.MakeInternalAPI("queryRoomsForUser", func(req *http.Request) util.JSONResponse {
			request := api.QueryRoomsForUserRequest{}
			response := api.QueryRoomsForUserResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.QueryRoomsForUser(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(RoomserverQueryBulkStateContentPath,
		httputil.MakeInternalAPI("queryBulkStateContent", func(req *http.Request) util.JSONResponse {
			request := api.QueryBulkStateContentRequest{}
			response := api.QueryBulkStateContentResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.QueryBulkStateContent(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(RoomserverQuerySharedUsersPath,
		httputil.MakeInternalAPI("querySharedUsers", func(req *http.Request) util.JSONResponse {
			request := api.QuerySharedUsersRequest{}
			response := api.QuerySharedUsersResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.QuerySharedUsers(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(RoomserverQueryKnownUsersPath,
		httputil.MakeInternalAPI("queryKnownUsers", func(req *http.Request) util.JSONResponse {
			request := api.QueryKnownUsersRequest{}
			response := api.QueryKnownUsersResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.QueryKnownUsers(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(RoomserverQueryServerBannedFromRoomPath,
		httputil.MakeInternalAPI("queryServerBannedFromRoom", func(req *http.Request) util.JSONResponse {
			request := api.QueryServerBannedFromRoomRequest{}
			response := api.QueryServerBannedFromRoomResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.QueryServerBannedFromRoom(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(RoomserverQueryAuthChainPath,
		httputil.MakeInternalAPI("queryAuthChain", func(req *http.Request) util.JSONResponse {
			request := api.QueryAuthChainRequest{}
			response := api.QueryAuthChainResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.QueryAuthChain(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
