package inthttp

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/util"
)

// AddRoutes adds the FederationSenderInternalAPI handlers to the http.ServeMux.
func AddRoutes(intAPI api.FederationSenderInternalAPI, internalAPIMux *mux.Router) {
	internalAPIMux.Handle(
		FederationSenderQueryJoinedHostServerNamesInRoomPath,
		httputil.MakeInternalAPI("QueryJoinedHostServerNamesInRoom", func(req *http.Request) util.JSONResponse {
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
	internalAPIMux.Handle(
		FederationSenderPerformJoinRequestPath,
		httputil.MakeInternalAPI("PerformJoinRequest", func(req *http.Request) util.JSONResponse {
			var request api.PerformJoinRequest
			var response api.PerformJoinResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			intAPI.PerformJoin(req.Context(), &request, &response)
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderPerformLeaveRequestPath,
		httputil.MakeInternalAPI("PerformLeaveRequest", func(req *http.Request) util.JSONResponse {
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
	internalAPIMux.Handle(
		FederationSenderPerformDirectoryLookupRequestPath,
		httputil.MakeInternalAPI("PerformDirectoryLookupRequest", func(req *http.Request) util.JSONResponse {
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
	internalAPIMux.Handle(
		FederationSenderPerformServersAlivePath,
		httputil.MakeInternalAPI("PerformServersAliveRequest", func(req *http.Request) util.JSONResponse {
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
	internalAPIMux.Handle(
		FederationSenderPerformBroadcastEDUPath,
		httputil.MakeInternalAPI("PerformBroadcastEDU", func(req *http.Request) util.JSONResponse {
			var request api.PerformBroadcastEDURequest
			var response api.PerformBroadcastEDUResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := intAPI.PerformBroadcastEDU(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
