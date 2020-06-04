package httpint

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/util"
)

// AddRoutes adds the FederationSenderInternalAPI handlers to the http.ServeMux.
func AddRoutes(intAPI api.FederationSenderInternalAPI, internalAPIMux *mux.Router) {
	internalAPIMux.Handle(
		FederationSenderQueryJoinedHostsInRoomPath,
		internal.MakeInternalAPI("QueryJoinedHostsInRoom", func(req *http.Request) util.JSONResponse {
			var request api.QueryJoinedHostsInRoomRequest
			var response api.QueryJoinedHostsInRoomResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := intAPI.QueryJoinedHostsInRoom(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderQueryJoinedHostServerNamesInRoomPath,
		internal.MakeInternalAPI("QueryJoinedHostServerNamesInRoom", func(req *http.Request) util.JSONResponse {
			var request api.QueryJoinedHostServerNamesInRoomRequest
			var response api.QueryJoinedHostServerNamesInRoomResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := intAPI.QueryJoinedHostServerNamesInRoom(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(FederationSenderPerformJoinRequestPath,
		internal.MakeInternalAPI("PerformJoinRequest", func(req *http.Request) util.JSONResponse {
			var request api.PerformJoinRequest
			var response api.PerformJoinResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := intAPI.PerformJoin(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(FederationSenderPerformLeaveRequestPath,
		internal.MakeInternalAPI("PerformLeaveRequest", func(req *http.Request) util.JSONResponse {
			var request api.PerformLeaveRequest
			var response api.PerformLeaveResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := intAPI.PerformLeave(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(FederationSenderPerformDirectoryLookupRequestPath,
		internal.MakeInternalAPI("PerformDirectoryLookupRequest", func(req *http.Request) util.JSONResponse {
			var request api.PerformDirectoryLookupRequest
			var response api.PerformDirectoryLookupResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := intAPI.PerformDirectoryLookup(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(FederationSenderPerformServersAlivePath,
		internal.MakeInternalAPI("PerformServersAliveRequest", func(req *http.Request) util.JSONResponse {
			var request api.PerformServersAliveRequest
			var response api.PerformServersAliveResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := intAPI.PerformServersAlive(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
